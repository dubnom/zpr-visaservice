// Package diagnostics owns production telemetry contracts, acquisition and aggregation.
package diagnostics

import (
	"context"
	"sort"
	"time"

	"neboagency.com/zpr-dashborad/internal/controlservice/inventory"
)

const (
	MaxSources      = 100
	MaxLogs         = 50
	MaxMetrics      = 50
	MaxTotalLogs    = 500
	MaxTotalMetrics = 1000
	MaxQueryBytes   = 512 << 10
)

type OTelIdentity struct {
	ServiceName string `json:"service_name"`
	InstanceID  string `json:"instance_id"`
}

type Provider interface {
	Query(context.Context, Source, int) ([]Log, []Metric, error)
}

type Source struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Kind        string     `json:"kind"`
	Identity    string     `json:"identity"`
	Address     string     `json:"address,omitempty"`
	ServiceName string     `json:"service_name"`
	InstanceID  string     `json:"service_instance_id"`
	State       string     `json:"state"`
	LastUpdated *time.Time `json:"last_updated,omitempty"`
	Logs        []Log      `json:"logs"`
	Metrics     []Metric   `json:"metrics"`
	Error       string     `json:"error,omitempty"`
}

type Log struct {
	Timestamp  time.Time      `json:"timestamp"`
	Severity   string         `json:"severity,omitempty"`
	Body       string         `json:"body"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

type Metric = inventory.Metric

type Response struct {
	GeneratedAt time.Time `json:"generated_at"`
	State       string    `json:"state"`
	Error       string    `json:"error,omitempty"`
	Sources     []Source  `json:"sources"`
}

// Sources filters an explicitly selected identity before applying aggregate caps.
func Sources(data inventory.Snapshot, mappings map[string]OTelIdentity, sourceID string) []Source {
	sources := make([]Source, 0, len(data.Actors)+len(data.Services)+len(data.Trusted))
	appendSource := func(id, name, kind, identity, address string) {
		if sourceID != "" && id != sourceID {
			return
		}
		mapping := mappings[id]
		serviceName := mapping.ServiceName
		instanceID := mapping.InstanceID
		if serviceName == "" {
			serviceName = identity
		}
		if instanceID == "" {
			instanceID = identity
		}
		sources = append(sources, Source{ID: id, Name: name, Kind: kind, Identity: identity, Address: address,
			ServiceName: serviceName, InstanceID: instanceID, State: "unavailable", Logs: []Log{}, Metrics: []Metric{}})
	}
	for _, actor := range data.Actors {
		if actor.Node {
			appendSource("node:"+actor.CN, actor.CN, "ZPR node", actor.CN, actor.ZPRAddress)
		}
	}
	for _, service := range data.Services {
		if service.Name == "" {
			continue
		}
		appendSource("service:"+service.Name, service.Name, "Required service · "+service.Kind, service.ActorCN, service.Address)
	}
	for _, trusted := range data.Trusted {
		identity := trusted.ActorCN
		if identity == "" {
			identity = trusted.Name
		}
		appendSource("trusted:"+trusted.Name, trusted.Name, "Trusted service · "+trusted.Provider, identity, trusted.ZPRAddress)
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Kind != sources[j].Kind {
			return sources[i].Kind < sources[j].Kind
		}
		return sources[i].Name < sources[j].Name
	})
	if len(sources) > MaxSources {
		sources = sources[:MaxSources]
	}
	return sources
}
