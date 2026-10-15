package inventory

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"

	"neboagency.com/zpr-dashborad/internal/controlservice/adminapi"
)

type Reader interface {
	GetJSON(context.Context, string, any) error
}

type Options struct {
	TrustedEditorURL func(name, provider string) string
	// Enrich runs after trusted inventory is built, before health is finalized.
	Enrich func(context.Context, *Snapshot)
}

// Fetch requires a configured reader and uses the caller's deadline unchanged.
func Fetch(ctx context.Context, reader Reader, options Options) Snapshot {
	out := Snapshot{
		GeneratedAt: time.Now().UTC(), APIStatus: "connected",
		Errors: []string{}, Stats: map[string]string{},
		Actors: []Actor{}, Network: []Link{}, Services: []Service{},
		Trusted: []TrustedSource{}, RecentVisas: []Visa{}, RecentDenies: []Deny{},
	}
	type result struct {
		name  string
		value any
		err   error
	}
	results := make(chan result, 7)
	go func() {
		var value StatsResponse
		err := reader.GetJSON(ctx, "/admin/stats", &value)
		results <- result{"stats", value.Stats, err}
	}()
	go func() {
		var value []ActorEntry
		err := reader.GetJSON(ctx, "/admin/actors", &value)
		results <- result{"actors", value, err}
	}()
	go func() {
		var value struct {
			Network []Link `json:"network"`
		}
		err := reader.GetJSON(ctx, "/admin/network", &value)
		results <- result{"network", value.Network, err}
	}()
	go func() {
		var value []ServiceEntry
		err := reader.GetJSON(ctx, "/admin/services", &value)
		results <- result{"services", value, err}
	}()
	go func() {
		var value []TrustedStatus
		err := reader.GetJSON(ctx, "/admin/trusted-services", &value)
		results <- result{"trusted", value, err}
	}()
	go func() {
		var value []VisaEntry
		err := reader.GetJSON(ctx, "/admin/visas", &value)
		results <- result{"visas", value, err}
	}()
	go func() {
		var value []Deny
		err := reader.GetJSON(ctx, "/admin/visas/denies?limit="+fmt.Sprint(MaxRecentDenies), &value)
		results <- result{"denies", value, err}
	}()
	var mu sync.Mutex
	successfulEndpoints := 0
	var statuses []TrustedStatus
	haveStatuses := false
	for i := 0; i < 7; i++ {
		res := <-results
		if res.err != nil {
			if res.name == "trusted" && notFound(res.err) {
				continue
			}
			mu.Lock()
			out.Errors = append(out.Errors, res.name+": "+res.err.Error())
			mu.Unlock()
			continue
		}
		successfulEndpoints++
		switch res.name {
		case "stats":
			out.Stats = res.value.(map[string]string)
		case "actors":
			out.Actors = FetchActors(ctx, reader, res.value.([]ActorEntry), &out.Errors, &mu)
		case "network":
			out.Network = res.value.([]Link)
		case "services":
			out.Services = FetchServices(ctx, reader, res.value.([]ServiceEntry), &out.Errors, &mu)
		case "trusted":
			statuses, haveStatuses = res.value.([]TrustedStatus), true
		case "visas":
			entries := res.value.([]VisaEntry)
			out.VisaCount = len(entries)
			items, complete := FetchVisaInventory(ctx, reader, entries, &out.Errors, &mu)
			out.RecentVisas = items[:min(len(items), MaxRecentVisas)]
			if complete {
				out.ActiveVisas = []Visa{}
				now := time.Now().Unix()
				for _, item := range items {
					if item.Expires > now {
						out.ActiveVisas = append(out.ActiveVisas, item)
					}
				}
			}
		case "denies":
			out.RecentDenies = res.value.([]Deny)
		}
	}
	if haveStatuses {
		out.Trusted = TrustedSourcesFromStatus(statuses, out.Services, options.TrustedEditorURL)
	} else {
		out.Trusted = TrustedSourcesFrom(out.Services)
	}
	if options.Enrich != nil {
		options.Enrich(ctx, &out)
	}
	if successfulEndpoints == 0 {
		out.APIStatus = "disconnected"
	} else if len(out.Errors) > 0 {
		out.APIStatus = "partial"
	}
	sort.Strings(out.Errors)
	return out
}

func notFound(err error) bool {
	var status adminapi.StatusError
	return errors.As(err, &status) && status.Code == http.StatusNotFound
}

func FetchActors(ctx context.Context, reader Reader, entries []ActorEntry, errs *[]string, mu *sync.Mutex) []Actor {
	unique := make([]ActorEntry, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if !seen[entry.CN] {
			seen[entry.CN] = true
			unique = append(unique, entry)
		}
	}
	entries = unique
	items := make([]Actor, len(entries))
	var wg sync.WaitGroup
	for i, entry := range entries {
		wg.Add(1)
		go func(i int, cn string) {
			defer wg.Done()
			var value Actor
			path := "/admin/actors/" + url.PathEscape(cn)
			if err := reader.GetJSON(ctx, path, &value); err != nil {
				mu.Lock()
				*errs = append(*errs, "actor "+cn+": "+err.Error())
				mu.Unlock()
				items[i] = Actor{CN: cn}
				return
			}
			items[i] = value
		}(i, entry.CN)
	}
	wg.Wait()
	sort.Slice(items, func(i, j int) bool { return items[i].CN < items[j].CN })
	return items
}

func FetchServices(ctx context.Context, reader Reader, entries []ServiceEntry, errs *[]string, mu *sync.Mutex) []Service {
	items := make([]Service, len(entries))
	var wg sync.WaitGroup
	for i, entry := range entries {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			path := "/admin/services/" + url.PathEscape(name)
			if err := reader.GetJSON(ctx, path, &items[i]); err != nil {
				mu.Lock()
				*errs = append(*errs, "service "+name+": "+err.Error())
				mu.Unlock()
			}
		}(i, entry.ID)
	}
	wg.Wait()
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func FetchVisaInventory(ctx context.Context, reader Reader, entries []VisaEntry, errs *[]string, mu *sync.Mutex) ([]Visa, bool) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID > entries[j].ID })
	items := make([]Visa, len(entries))
	valid := make([]bool, len(entries))
	complete := true
	var wg sync.WaitGroup
	workers := make(chan struct{}, 16)
	for i, entry := range entries {
		wg.Add(1)
		go func(i int, id int64) {
			defer wg.Done()
			select {
			case workers <- struct{}{}:
				defer func() { <-workers }()
			case <-ctx.Done():
				mu.Lock()
				complete = false
				*errs = append(*errs, fmt.Sprintf("visa %d: %v", id, ctx.Err()))
				mu.Unlock()
				return
			}
			path := "/admin/visas/" + fmt.Sprint(id)
			if err := reader.GetJSON(ctx, path, &items[i]); err != nil {
				if notFound(err) {
					return
				}
				mu.Lock()
				complete = false
				*errs = append(*errs, fmt.Sprintf("visa %d: %v", id, err))
				mu.Unlock()
			} else {
				valid[i] = true
			}
		}(i, entry.ID)
	}
	wg.Wait()
	result := make([]Visa, 0, len(items))
	for index, item := range items {
		if valid[index] {
			result = append(result, item)
		}
	}
	return result, complete
}
