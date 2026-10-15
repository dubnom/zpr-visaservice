package inventory

import (
	"sort"
	"strings"
)

func MergePlatformServices(out *Snapshot, configured []Service) {
	serviceIndexes := make(map[string]int, len(out.Services))
	for index, item := range out.Services {
		serviceIndexes[item.Name] = index
	}
	for _, item := range configured {
		if item.Name == "" || item.ActorCN == "" {
			continue
		}
		if index, exists := serviceIndexes[item.Name]; exists {
			if strings.EqualFold(item.Kind, "Gateway") || item.ExternalNetworkConnection != "" {
				out.Services[index].Kind = "Gateway"
				out.Services[index].ExternalNetworkConnection = item.ExternalNetworkConnection
			}
			continue
		}
		out.Services = append(out.Services, item)
		serviceIndexes[item.Name] = len(out.Services) - 1
	}
	sort.Slice(out.Services, func(i, j int) bool { return out.Services[i].Name < out.Services[j].Name })
	sort.Slice(out.Actors, func(i, j int) bool { return out.Actors[i].CN < out.Actors[j].CN })
}

func TrustedSourcesFrom(services []Service) []TrustedSource {
	var trusted []TrustedSource
	for _, item := range services {
		if !strings.HasPrefix(item.Kind, "Trusted(") {
			continue
		}
		provider := strings.TrimSuffix(strings.TrimPrefix(item.Kind, "Trusted(\""), "\")")
		if provider == item.Kind {
			provider = "trusted"
		}
		trusted = append(trusted, TrustedSource{
			Name: item.Name, ActorCN: item.ActorCN, Provider: provider,
			ZPRAddress: item.Address, Endpoints: item.Endpoints,
			Health: "unreported", HealthNote: "The admin API exposes no live connection or source-health signal.",
		})
	}
	return trusted
}

func TrustedSourcesFromStatus(statuses []TrustedStatus, services []Service, editorURL func(name, provider string) string) []TrustedSource {
	byName := make(map[string]Service, len(services))
	for _, item := range services {
		byName[item.Name] = item
	}
	trusted := make([]TrustedSource, 0, len(statuses))
	for _, status := range statuses {
		provider := "trusted source"
		actor := ""
		address := ""
		endpoints := ""
		if descriptor, ok := byName[status.Name]; ok {
			actor = descriptor.ActorCN
			address = descriptor.Address
			endpoints = descriptor.Endpoints
			provider = strings.TrimSuffix(strings.TrimPrefix(descriptor.Kind, "Trusted(\""), "\")")
			if provider == descriptor.Kind {
				provider = "trusted source"
			}
		}
		health := status.Health
		if health != "working" && health != "failed" {
			health = "unverified"
		}
		var manager string
		if editorURL != nil {
			manager = editorURL(status.Name, provider)
		}
		trusted = append(trusted, TrustedSource{
			Name: status.Name, ActorCN: actor, Provider: provider, Health: health,
			ZPRAddress: address, Endpoints: endpoints,
			HealthNote:   "Based on the most recent real attribute lookup, not a live probe.",
			LastLookupMS: status.LastLookupMS, LastSuccessMS: status.LastSuccessMS,
			EditorURL: manager,
		})
	}
	sort.Slice(trusted, func(i, j int) bool { return trusted[i].Name < trusted[j].Name })
	return trusted
}
