package changewatcher

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
)

type Event struct {
	Version        int    `json:"version"`
	BaseDN         string `json:"base_dn"`
	Type           string `json:"type"`
	UUID           string `json:"uuid,omitempty"`
	DN             string `json:"dn,omitempty"`
	RefreshDeletes bool   `json:"refresh_deletes,omitempty"`
}

// WatchLDAP subscribes to RFC 4533 without exposing entry values or credentials.
func WatchLDAP(ctx context.Context, uri, caFile, baseDN, bindDN, passwordFile string, initial []byte, emit Sink, checkpoint func([]byte) error) error {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "ldaps" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return errors.New("LDAP change consumer requires an ldaps://host URI")
	}
	if baseDN == "" || bindDN == "" || caFile == "" || passwordFile == "" {
		return errors.New("LDAP change consumer requires base, bind DN, CA and password file")
	}
	if _, err := ldap.ParseDN(baseDN); err != nil {
		return errors.New("invalid LDAP change consumer base DN")
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return errors.New("cannot read LDAP change consumer CA")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return errors.New("invalid LDAP change consumer CA")
	}
	password, err := os.ReadFile(passwordFile)
	if err != nil {
		return errors.New("cannot read LDAP change consumer password file")
	}
	if len(strings.TrimSuffix(strings.TrimSuffix(string(password), "\n"), "\r")) == 0 {
		return errors.New("LDAP change consumer requires a nonempty password")
	}
	sink := emit
	emit = func(event Event) error {
		event.Version, event.BaseDN = 1, baseDN
		return sink(event)
	}
	dial := func(cookie []byte) (ldap.Response, func(), error) {
		conn, err := ldap.DialURL(uri, ldap.DialWithTLSConfig(&tls.Config{
			RootCAs: roots, ServerName: parsed.Hostname(), MinVersion: tls.VersionTLS12,
		}), ldap.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}))
		if err != nil {
			return nil, nil, err
		}
		conn.SetTimeout(5 * time.Second)
		if err := conn.Bind(bindDN, strings.TrimSuffix(strings.TrimSuffix(string(password), "\n"), "\r")); err != nil {
			conn.Close()
			return nil, nil, err
		}
		conn.SetTimeout(0)
		request := ldap.NewSearchRequest(baseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false, "(objectClass=*)", []string{"1.1"}, nil)
		return conn.Syncrepl(ctx, request, 16, ldap.SyncRequestModeRefreshAndPersist, cookie, false), func() { conn.Close() }, nil
	}
	return ConsumeLDAPChanges(ctx, dial, emit, initial, checkpoint)
}

type SyncDial func([]byte) (ldap.Response, func(), error)
type Sink func(Event) error

// ConsumeLDAPChanges resumes disconnects and reports refreshes explicitly to the sink.
func ConsumeLDAPChanges(ctx context.Context, dial SyncDial, emit Sink, initial []byte, checkpoint func([]byte) error) error {
	cookie := append([]byte(nil), initial...)
	backoff := time.Second
	for ctx.Err() == nil {
		start := "refresh_start"
		if len(cookie) > 0 {
			start = "resume_start"
		}
		if err := emit(Event{Type: start}); err != nil {
			return err
		}
		response, closeConnection, err := dial(cookie)
		if err == nil {
			var outputErr error
			for response.Next() {
				if ctx.Err() != nil {
					break
				}
				updated, processErr := ConsumeLDAPSyncResponse(response, emit)
				if processErr != nil {
					outputErr = processErr
					break
				}
				if len(updated) > 0 {
					if checkpoint != nil {
						if err := checkpoint(updated); err != nil {
							outputErr = err
							break
						}
					}
					cookie = append(cookie[:0], updated...)
					backoff = time.Second
				}
			}
			err = response.Err()
			closeConnection()
			if outputErr != nil {
				return outputErr
			}
		}
		if ctx.Err() != nil {
			return nil
		}
		if ldap.IsErrorWithCode(err, 4096) {
			if checkpoint != nil {
				if err := checkpoint(nil); err != nil {
					return err
				}
			}
			cookie = nil
			if err := emit(Event{Type: "resync_required"}); err != nil {
				return err
			}
		}
		if err := emit(Event{Type: "disconnected"}); err != nil {
			return err
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
		backoff = min(backoff*2, 30*time.Second)
	}
	return nil
}

// ConsumeLDAPSyncResponse preserves RFC refresh/present/delete distinctions.
func ConsumeLDAPSyncResponse(response ldap.Response, emit Sink) ([]byte, error) {
	var cookie []byte
	if response.Entry() != nil && len(response.Controls()) == 0 {
		return nil, errors.New("LDAP entry is missing sync controls")
	}
	for _, control := range response.Controls() {
		switch control := control.(type) {
		case *ldap.ControlSyncState:
			if control == nil || len(control.Cookie) > maxCookie {
				return nil, errors.New("invalid LDAP sync state")
			}
			types := map[ldap.ControlSyncStateState]string{
				ldap.SyncStatePresent: "present", ldap.SyncStateAdd: "add",
				ldap.SyncStateModify: "modify", ldap.SyncStateDelete: "delete",
			}
			eventType, ok := types[control.State]
			if !ok {
				return nil, errors.New("unsupported LDAP sync state")
			}
			event := Event{Type: eventType, UUID: control.EntryUUID.String()}
			if entry := response.Entry(); entry != nil {
				event.DN = entry.DN
			}
			if err := emit(event); err != nil {
				return nil, err
			}
			cookie = control.Cookie
		case *ldap.ControlSyncDone:
			if control == nil || len(control.Cookie) > maxCookie {
				return nil, errors.New("invalid LDAP sync completion")
			}
			if err := emit(Event{Type: "refresh_complete", RefreshDeletes: control.RefreshDeletes}); err != nil {
				return nil, err
			}
			cookie = control.Cookie
		case *ldap.ControlSyncInfo:
			if control == nil {
				return nil, errors.New("invalid LDAP sync info")
			}
			switch control.Value {
			case ldap.SyncInfoNewcookie:
				if control.NewCookie == nil || len(control.NewCookie.Cookie) > maxCookie {
					return nil, errors.New("invalid LDAP sync info payload")
				}
				cookie = control.NewCookie.Cookie
			case ldap.SyncInfoRefreshDelete:
				if control.RefreshDelete == nil || len(control.RefreshDelete.Cookie) > maxCookie {
					return nil, errors.New("invalid LDAP sync info payload")
				}
				cookie = control.RefreshDelete.Cookie
				if control.RefreshDelete.RefreshDone {
					if err := emit(Event{Type: "refresh_complete", RefreshDeletes: true}); err != nil {
						return nil, err
					}
				}
			case ldap.SyncInfoRefreshPresent:
				if control.RefreshPresent == nil || len(control.RefreshPresent.Cookie) > maxCookie {
					return nil, errors.New("invalid LDAP sync info payload")
				}
				cookie = control.RefreshPresent.Cookie
				if control.RefreshPresent.RefreshDone {
					if err := emit(Event{Type: "refresh_complete"}); err != nil {
						return nil, err
					}
				}
			case ldap.SyncInfoSyncIdSet:
				if control.SyncIdSet == nil || len(control.SyncIdSet.Cookie) > maxCookie {
					return nil, errors.New("invalid LDAP sync info payload")
				}
				if len(control.SyncIdSet.SyncUUIDs) > 4096 {
					return nil, errors.New("LDAP sync identity set exceeds limit")
				}
				cookie = control.SyncIdSet.Cookie
				for _, entryUUID := range control.SyncIdSet.SyncUUIDs {
					eventType := "present"
					if control.SyncIdSet.RefreshDeletes {
						eventType = "delete"
					}
					if err := emit(Event{Type: eventType, UUID: entryUUID.String()}); err != nil {
						return nil, err
					}
				}
			default:
				return nil, fmt.Errorf("unsupported LDAP sync info type %d", control.Value)
			}
		}
	}
	return cookie, nil
}
