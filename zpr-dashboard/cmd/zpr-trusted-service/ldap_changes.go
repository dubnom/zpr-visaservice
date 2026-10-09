package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
)

const (
	accessLogTimeLayout   = "20060102150405.000000Z"
	changeCursorPrefix    = "v1:"
	defaultChangeLimit    = 100
	maxChangeLimit        = 500
	changeWindowSizeLimit = 500
)

var changeLookbackPattern = regexp.MustCompile(`^[0-9]+(h|m|s)$`)

var errChangeCursorExpired = errors.New("change cursor is older than the retained change log")
var errChangeLookbackExpired = errors.New("change lookback is older than the retained change log")
var errChangeLookbackFuture = errors.New("change lookback is in the future")

type ldapChange struct {
	Cursor     string    `json:"cursor"`
	Time       time.Time `json:"time"`
	Type       string    `json:"type"`
	DN         string    `json:"dn"`
	NewDN      string    `json:"new_dn,omitempty"`
	EntryUUID  string    `json:"entry_uuid,omitempty"`
	Attributes []string  `json:"attributes,omitempty"`
}

type changesResponse struct {
	Changes []ldapChange `json:"changes"`
	Cursor  string       `json:"cursor"`
	More    bool         `json:"more"`
}

type changeSource interface {
	changes(ctx context.Context, cursor time.Time, limit int) (changesResponse, error)
	changesSince(ctx context.Context, since time.Time, limit int) (changesResponse, error)
	changesWithin(ctx context.Context, lookback time.Duration, limit int) (changesResponse, error)
	head() time.Time
}

// accessLogChanges reads committed writes from an OpenLDAP accesslog database.
// Window bounds are (cursor, now-settle]; the settle delay lets writes that started
// earlier finish logging before the cursor moves past them.
type accessLogChanges struct {
	logBase   string
	retention time.Duration
	settle    time.Duration
	now       func() time.Time
	connect   func(context.Context) (ldapSearcher, func(), error)
}

// head is the end of the last whole second before the settle delay. slapd's reqStart
// fraction is a per-second sequence counter, not wall-clock microseconds, so a window
// ending mid-second could skip writes logged later in that second.
func (c accessLogChanges) head() time.Time {
	return c.now().UTC().Add(-c.settle).Truncate(time.Second).Add(-time.Microsecond)
}

func (c accessLogChanges) changes(ctx context.Context, cursor time.Time, limit int) (changesResponse, error) {
	return c.readChanges(ctx, cursor, limit, true)
}

func (c accessLogChanges) changesSince(ctx context.Context, since time.Time, limit int) (changesResponse, error) {
	now := c.now().UTC()
	if since.Before(now.Add(-c.retention)) {
		return changesResponse{}, errChangeLookbackExpired
	}
	if since.After(c.head()) {
		return changesResponse{}, errChangeLookbackFuture
	}
	// Accesslog fractions are sequence stamps within a second, so start just
	// before the requested second to include every write in that second.
	cursor := since.UTC().Truncate(time.Second).Add(-time.Microsecond)
	return c.readChanges(ctx, cursor, limit, false)
}

func (c accessLogChanges) changesWithin(ctx context.Context, lookback time.Duration, limit int) (changesResponse, error) {
	if lookback <= 0 || lookback > c.retention {
		return changesResponse{}, errChangeLookbackExpired
	}
	cursor := c.now().UTC().Add(-lookback).Truncate(time.Second).Add(-time.Microsecond)
	return c.readChanges(ctx, cursor, limit, false)
}

func (c accessLogChanges) readChanges(ctx context.Context, cursor time.Time, limit int, validateAnchor bool) (changesResponse, error) {
	upper := c.head()
	if !cursor.Before(upper) {
		return changesResponse{Changes: []ldapChange{}, Cursor: encodeChangeCursor(cursor)}, nil
	}
	searcher, closeConnection, err := c.connect(ctx)
	if err != nil {
		return changesResponse{}, err
	}
	defer closeConnection()
	if validateAnchor && cursor.Before(c.now().UTC().Add(-c.retention)) {
		anchored, err := c.entryExists(searcher, cursor)
		if err != nil {
			return changesResponse{}, err
		}
		if !anchored {
			return changesResponse{}, errChangeCursorExpired
		}
	}
	hi := upper
	var entries []*ldap.Entry
	for {
		if err := ctx.Err(); err != nil {
			return changesResponse{}, err
		}
		result, err := searcher.Search(ldap.NewSearchRequest(c.logBase, ldap.ScopeSingleLevel, ldap.NeverDerefAliases,
			changeWindowSizeLimit, 10, false, accessLogWindowFilter(cursor, hi),
			[]string{"reqStart", "reqType", "reqDN", "reqEntryUUID", "reqMod", "reqNewRDN", "reqNewSuperior"}, nil))
		if ldap.IsErrorWithCode(err, ldap.LDAPResultSizeLimitExceeded) {
			// Results are unordered, so a truncated window cannot be paged; halve it instead.
			span := hi.Sub(cursor) / 2
			if span < time.Microsecond {
				return changesResponse{}, errors.New("change log window cannot be narrowed below the server size limit")
			}
			hi = cursor.Add(span).Truncate(time.Microsecond)
			continue
		}
		if err != nil {
			return changesResponse{}, err
		}
		entries = result.Entries
		break
	}
	changes := make([]ldapChange, 0, len(entries))
	for _, entry := range entries {
		change, err := accessLogChange(entry)
		if err != nil {
			return changesResponse{}, err
		}
		changes = append(changes, change)
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].Time.Before(changes[j].Time) })
	response := changesResponse{Changes: changes, Cursor: encodeChangeCursor(hi), More: hi.Before(upper)}
	if len(changes) > limit {
		response.Changes = changes[:limit]
		response.Cursor = response.Changes[limit-1].Cursor
		response.More = true
	}
	return response, nil
}

func (c accessLogChanges) entryExists(searcher ldapSearcher, at time.Time) (bool, error) {
	filter := fmt.Sprintf("(&(objectClass=auditWriteObject)(reqStart=%s))", at.UTC().Format(accessLogTimeLayout))
	result, err := searcher.Search(ldap.NewSearchRequest(c.logBase, ldap.ScopeSingleLevel, ldap.NeverDerefAliases,
		1, 10, false, filter, []string{"1.1"}, nil))
	if err != nil {
		return false, err
	}
	return len(result.Entries) > 0, nil
}

func accessLogWindowFilter(after, through time.Time) string {
	lo := after.UTC().Format(accessLogTimeLayout)
	return fmt.Sprintf("(&(objectClass=auditWriteObject)(reqResult=0)(reqStart>=%s)(!(reqStart=%s))(reqStart<=%s))",
		lo, lo, through.UTC().Format(accessLogTimeLayout))
}

var operationalAttributes = map[string]bool{
	"createtimestamp": true, "creatorsname": true, "entrycsn": true, "entrydn": true, "entryuuid": true,
	"hassubordinates": true, "modifiersname": true, "modifytimestamp": true, "structuralobjectclass": true,
	"subschemasubentry": true, "contextcsn": true,
}

// accessLogChange converts an audit entry to metadata; reqMod values are never copied.
func accessLogChange(entry *ldap.Entry) (ldapChange, error) {
	start, err := time.Parse(accessLogTimeLayout, entry.GetAttributeValue("reqStart"))
	if err != nil {
		return ldapChange{}, errors.New("invalid accesslog reqStart")
	}
	change := ldapChange{
		Cursor:    encodeChangeCursor(start),
		Time:      start,
		Type:      entry.GetAttributeValue("reqType"),
		DN:        entry.GetAttributeValue("reqDN"),
		EntryUUID: entry.GetAttributeValue("reqEntryUUID"),
	}
	switch change.Type {
	case "add", "delete", "modify", "modrdn":
	default:
		return ldapChange{}, fmt.Errorf("unsupported accesslog reqType %q", change.Type)
	}
	seen := map[string]bool{}
	for _, mod := range entry.GetAttributeValues("reqMod") {
		name, _, ok := strings.Cut(mod, ":")
		lower := strings.ToLower(name)
		if !ok || !safeLDAPAttribute(name) || operationalAttributes[lower] || seen[lower] {
			continue
		}
		seen[lower] = true
		change.Attributes = append(change.Attributes, name)
	}
	sort.Strings(change.Attributes)
	if change.Type == "modrdn" {
		if newRDN := entry.GetAttributeValue("reqNewRDN"); newRDN != "" {
			parent := entry.GetAttributeValue("reqNewSuperior")
			if parent == "" {
				if _, rest, found := strings.Cut(change.DN, ","); found {
					parent = rest
				}
			}
			change.NewDN = newRDN
			if parent != "" {
				change.NewDN += "," + parent
			}
		}
	}
	return change, nil
}

func encodeChangeCursor(at time.Time) string {
	return base64.RawURLEncoding.EncodeToString([]byte(changeCursorPrefix + at.UTC().Format(accessLogTimeLayout)))
}

func decodeChangeCursor(cursor string) (time.Time, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil || !strings.HasPrefix(string(raw), changeCursorPrefix) {
		return time.Time{}, errors.New("invalid cursor")
	}
	at, err := time.Parse(accessLogTimeLayout, strings.TrimPrefix(string(raw), changeCursorPrefix))
	if err != nil {
		return time.Time{}, errors.New("invalid cursor")
	}
	return at, nil
}

func changesHandler(source changeSource) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		for name := range query {
			if name != "cursor" && name != "limit" && name != "since" {
				http.Error(w, "invalid changes request", http.StatusBadRequest)
				return
			}
		}
		if query.Has("cursor") && query.Has("since") {
			http.Error(w, "cursor and since are mutually exclusive", http.StatusBadRequest)
			return
		}
		limit := defaultChangeLimit
		if value := query.Get("limit"); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 1 || parsed > maxChangeLimit {
				http.Error(w, "invalid limit", http.StatusBadRequest)
				return
			}
			limit = parsed
		}
		w.Header().Set("Cache-Control", "no-store")
		if !query.Has("cursor") {
			if query.Has("since") {
				sinceText := query.Get("since")
				lookback, durationErr := time.ParseDuration(sinceText)
				var response changesResponse
				var err error
				if durationErr == nil {
					if !changeLookbackPattern.MatchString(sinceText) {
						http.Error(w, "invalid since", http.StatusBadRequest)
						return
					}
					response, err = source.changesWithin(r.Context(), lookback, limit)
				} else {
					since, parseErr := time.Parse(time.RFC3339Nano, sinceText)
					if parseErr != nil {
						http.Error(w, "invalid since", http.StatusBadRequest)
						return
					}
					response, err = source.changesSince(r.Context(), since, limit)
				}
				if errors.Is(err, errChangeCursorExpired) || errors.Is(err, errChangeLookbackExpired) {
					writeChangesJSON(w, http.StatusGone, map[string]string{"error": "cursor_expired"})
					return
				}
				if errors.Is(err, errChangeLookbackFuture) {
					http.Error(w, "invalid since", http.StatusBadRequest)
					return
				}
				if err != nil {
					logChangeFailure(err)
					http.Error(w, "change log unavailable", http.StatusServiceUnavailable)
					return
				}
				writeChangesJSON(w, http.StatusOK, response)
				return
			}
			writeChangesJSON(w, http.StatusOK, changesResponse{Changes: []ldapChange{}, Cursor: encodeChangeCursor(source.head())})
			return
		}
		cursor, err := decodeChangeCursor(query.Get("cursor"))
		if err != nil {
			http.Error(w, "invalid cursor", http.StatusBadRequest)
			return
		}
		response, err := source.changes(r.Context(), cursor, limit)
		if errors.Is(err, errChangeCursorExpired) {
			writeChangesJSON(w, http.StatusGone, map[string]string{"error": "cursor_expired"})
			return
		}
		if err != nil {
			logChangeFailure(err)
			http.Error(w, "change log unavailable", http.StatusServiceUnavailable)
			return
		}
		writeChangesJSON(w, http.StatusOK, response)
	}
}
