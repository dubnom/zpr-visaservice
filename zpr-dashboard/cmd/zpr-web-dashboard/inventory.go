package main

import (
	"context"
	"sync"

	"neboagency.com/zpr-dashborad/internal/controlservice/inventory"
)

const (
	maxRecentVisas  = inventory.MaxRecentVisas
	maxRecentDenies = inventory.MaxRecentDenies
)

type snapshot = inventory.Snapshot
type actorEntry = inventory.ActorEntry
type actor = inventory.Actor
type nodeDetail = inventory.NodeDetail
type nodeCounter = inventory.NodeCounter
type link = inventory.Link
type serviceEntry = inventory.ServiceEntry
type service = inventory.Service
type trustedSource = inventory.TrustedSource
type trustedStatus = inventory.TrustedStatus
type visaEntry = inventory.VisaEntry
type visa = inventory.Visa
type deny = inventory.Deny
type statsResponse = inventory.StatsResponse

func (a *application) fetchSnapshot(ctx context.Context) snapshot {
	return inventory.Fetch(ctx, a.admin.reader(), inventory.Options{
		TrustedEditorURL: trustedEditorURL,
		Enrich: func(ctx context.Context, out *snapshot) {
			mergePlatformServices(out)
			populateNodeDisplayNames(out)
			provider, staleAfter, providerError := newNodeDenialProvider()
			populateNodeDenialStats(ctx, out, provider, staleAfter, providerError, diagnosticsSourceMappings())
		},
	})
}

func mergePlatformServices(out *snapshot) {
	inventory.MergePlatformServices(out, platformServices())
}

func (a *application) fetchActors(ctx context.Context, entries []actorEntry, errs *[]string, mu *sync.Mutex) []actor {
	return inventory.FetchActors(ctx, a.admin.reader(), entries, errs, mu)
}

func (a *application) fetchServices(ctx context.Context, entries []serviceEntry, errs *[]string, mu *sync.Mutex) []service {
	return inventory.FetchServices(ctx, a.admin.reader(), entries, errs, mu)
}

func (a *application) fetchVisaInventory(ctx context.Context, entries []visaEntry, errs *[]string, mu *sync.Mutex) ([]visa, bool) {
	return inventory.FetchVisaInventory(ctx, a.admin.reader(), entries, errs, mu)
}

func trustedSourcesFrom(services []service) []trustedSource {
	return inventory.TrustedSourcesFrom(services)
}

func trustedSourcesFromStatus(statuses []trustedStatus, services []service) []trustedSource {
	return inventory.TrustedSourcesFromStatus(statuses, services, trustedEditorURL)
}

func trustedEditorURL(name, provider string) string {
	if manager := providerManagerURL(name); manager != "" {
		return manager
	}
	if name == "demo_ldap" && provider == "rest/1" {
		return localLDAPEditorURL()
	}
	return ""
}
