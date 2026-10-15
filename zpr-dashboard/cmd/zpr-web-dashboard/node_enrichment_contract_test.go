package main

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"
)

type nodeEnrichmentContractProvider struct {
	context context.Context
	sources []diagnosticsSource
	limits  []int
	t       *testing.T
}

func (p *nodeEnrichmentContractProvider) query(ctx context.Context, source diagnosticsSource, limit int) ([]diagnosticsLog, []diagnosticsMetric, error) {
	if ctx != p.context {
		p.t.Error("node enrichment changed the request context")
	}
	p.sources = append(p.sources, source)
	p.limits = append(p.limits, limit)
	return nil, nil, nil
}

func TestNodeEnrichmentPreservesDiagnosticsCatalogContract(t *testing.T) {
	unavailableSimulatorForProductionContract(t)
	data := snapshot{APIStatus: "connected", Errors: []string{}}
	for index := 100; index >= 0; index-- {
		data.Actors = append(data.Actors, actor{
			CN: fmt.Sprintf("node-%03d", index), Node: true,
			ZPRAddress: "fd00::1", NodeDetails: &nodeDetail{},
		})
	}
	mappings := map[string]diagnosticsOTelIdentity{
		"node:node-000": {ServiceName: "production-node", InstanceID: "instance-000"},
	}
	expected := diagnosticsSourcesFromSnapshot(data, mappings)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider := &nodeEnrichmentContractProvider{context: ctx, t: t}
	populateNodeDenialStats(ctx, &data, provider, time.Minute, "", mappings)
	if len(expected) != 100 || len(provider.sources) != 100 {
		t.Fatalf("catalog cap changed: catalog=%d queries=%d", len(expected), len(provider.sources))
	}
	slices.Reverse(expected)
	if !reflect.DeepEqual(provider.sources, expected) {
		t.Fatal("node query order or complete diagnostics source fields changed")
	}
	for _, limit := range provider.limits {
		if limit != 1 {
			t.Fatalf("node query limit changed: %d", limit)
		}
	}
	if got := data.Actors[0].NodeDetails; got.DenialStatsError != "Node telemetry identity unavailable." || got.CounterStatsError != got.DenialStatsError {
		t.Fatalf("node outside capped catalog must not query an invented identity: %+v", got)
	}
	if data.APIStatus != "connected" || len(data.Errors) != 0 {
		t.Fatal("telemetry failures changed inventory health")
	}
}
