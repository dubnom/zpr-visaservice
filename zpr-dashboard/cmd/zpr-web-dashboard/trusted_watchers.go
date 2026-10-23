package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"neboagency.com/zpr-dashborad/internal/controlservice/changewatcher"
)

type trustedWatcher struct {
	organization string
	source       string
	worker       *changewatcher.Worker
}

func validateWatchedSource(entry assertionSourceConfig) error {
	if entry.Kind != "ldap" || entry.URL == "" || entry.Container != "" || entry.CAFile == "" || len(entry.Organizations) == 0 {
		return errors.New("Watched sources require network LDAP, an explicit CA and organization scopes")
	}
	for organization, scope := range entry.Organizations {
		if strings.TrimSpace(organization) == "" || !assertionSourceName.MatchString(scope.VisaServiceID) {
			return errors.New("Watched source organizations require an explicit Visa Service trusted-service ID")
		}
	}
	return nil
}

// startTrustedWatchers owns all source checkpoints, even when Policy-Service or
// the operator UI is unavailable. Source changes require a controlled restart.
func (runtime *assertionRuntime) startTrustedWatchers(parent context.Context, dir string, admin *adminClient) (func(), error) {
	return runtime.startTrustedWatchersUsing(parent, dir, admin, trustedLDAPStream)
}

func trustedLDAPStream(entry assertionSourceConfig, scope assertionSourceScope) changewatcher.Stream {
	return func(ctx context.Context, initial []byte, sink changewatcher.Sink, checkpoint func([]byte) error) error {
		return changewatcher.WatchLDAP(ctx, entry.URL, entry.CAFile, scope.BaseDN, scope.BindDN, scope.BindPasswordFile, initial, sink, checkpoint)
	}
}

func (runtime *assertionRuntime) startTrustedWatchersUsing(parent context.Context, dir string, admin *adminClient, factory func(assertionSourceConfig, assertionSourceScope) changewatcher.Stream) (func(), error) {
	ctx, cancel := context.WithCancel(parent)
	var wait sync.WaitGroup
	var stores []*changewatcher.Store
	cleanup := func() {
		cancel()
		wait.Wait()
		for _, store := range stores {
			_ = store.Close()
		}
	}
	type pending struct {
		watcher trustedWatcher
		store   *changewatcher.Store
		cookie  []byte
		stream  changewatcher.Stream
	}
	var jobs []pending
	for _, entry := range runtime.sourcesConfig.Sources {
		if !entry.WatchChanges {
			continue
		}
		if err := validateWatchedSource(entry); err != nil {
			cleanup()
			return nil, err
		}
		if admin == nil {
			cleanup()
			return nil, errors.New("Watched sources require the production Visa Service Admin API")
		}
		source := runtime.sources[entry.Name]
		if source.read == nil {
			cleanup()
			return nil, errors.New("Watched source reader is unavailable")
		}
		for organization, scope := range entry.Organizations {
			// A rebind must explicitly resynchronize, never silently resume another source.
			identityEntry := entry
			identityEntry.Organizations = nil
			identity, _ := json.Marshal(struct {
				Entry        assertionSourceConfig
				Organization string
				Scope        assertionSourceScope
				AdminURL     string
			}{identityEntry, organization, scope, admin.baseURL})
			digest := sha256.Sum256(identity)
			store, cookie, err := changewatcher.OpenStore(dir, organization, entry.Name, hex.EncodeToString(digest[:]))
			if err != nil {
				cleanup()
				return nil, err
			}
			stores = append(stores, store)
			worker, err := changewatcher.NewWorker(scope.BaseDN, func(ctx context.Context) error {
				readCtx, stop := context.WithTimeout(ctx, 15*time.Second)
				defer stop()
				directory, err := source.read(readCtx, organization, scope.BaseDN, scope.BindDN)
				if err != nil {
					return errors.New("trusted source refresh failed")
				}
				if err := admin.refreshTrustedService(readCtx, scope.VisaServiceID); err != nil {
					return err
				}
				runtime.mu.Lock()
				if runtime.organizationID == organization {
					runtime.lastSource = assertionSourceSummary(directory, time.Now().UTC())
					runtime.lastSource["source_id"] = entry.Name
				}
				runtime.mu.Unlock()
				return nil
			})
			if err != nil {
				cleanup()
				return nil, err
			}
			stream := factory(entry, scope)
			jobs = append(jobs, pending{trustedWatcher{organization, entry.Name, worker}, store, cookie, stream})
		}
	}
	for _, job := range jobs {
		runtime.watchers = append(runtime.watchers, job.watcher)
		wait.Add(1)
		go func() { defer wait.Done(); job.watcher.worker.Run(ctx, job.store, job.cookie, job.stream) }()
	}
	return cleanup, nil
}

func (runtime *assertionRuntime) watcherStatus() []map[string]any {
	if runtime.organizationID == "" {
		return []map[string]any{}
	}
	return runtime.scopedWatcherStatus(runtime.organizationID)
}

func (runtime *assertionRuntime) allWatcherStatus() []map[string]any {
	return runtime.scopedWatcherStatus("")
}

func (runtime *assertionRuntime) scopedWatcherStatus(organization string) []map[string]any {
	result := []map[string]any{}
	for _, watcher := range runtime.watchers {
		if organization == "" || watcher.organization == organization {
			result = append(result, map[string]any{"organization_id": watcher.organization, "source_id": watcher.source, "health": watcher.worker.Status()})
		}
	}
	return result
}

func (client *adminClient) refreshTrustedService(ctx context.Context, serviceID string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, client.baseURL+"/admin/services/"+url.PathEscape(serviceID)+"/cache", nil)
	if err != nil {
		return errors.New("trusted service refresh request is invalid")
	}
	request.Header.Set("X-API-Key", client.apiKey)
	// Never forward the write-capable Admin credential across a redirect.
	transport := *client.http
	transport.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := transport.Do(request)
	if err != nil {
		return errors.New("trusted service cache refresh is unavailable")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("trusted service cache refresh rejected (HTTP %d)", response.StatusCode)
	}
	return nil
}
