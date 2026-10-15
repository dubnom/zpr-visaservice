// Package inventory owns production Admin inventory acquisition and wire types.
package inventory

import "time"

const (
	MaxRecentVisas  = 10
	MaxRecentDenies = 20
)

type Snapshot struct {
	GeneratedAt  time.Time         `json:"generated_at"`
	APIStatus    string            `json:"api_status"`
	ConfigError  string            `json:"config_error,omitempty"`
	Errors       []string          `json:"errors"`
	Stats        map[string]string `json:"stats"`
	Actors       []Actor           `json:"actors"`
	Network      []Link            `json:"network"`
	Services     []Service         `json:"services"`
	Trusted      []TrustedSource   `json:"trusted_sources"`
	VisaCount    int               `json:"visa_count"`
	RecentVisas  []Visa            `json:"recent_visas"`
	ActiveVisas  []Visa            `json:"active_visas"`
	RecentDenies []Deny            `json:"recent_denies"`
}

type ActorEntry struct {
	CN string `json:"cn"`
}

type Actor struct {
	CN          string      `json:"cn"`
	DisplayName string      `json:"display_name,omitempty"`
	Node        bool        `json:"node"`
	ZPRAddress  string      `json:"zpr_addr"`
	AuthExpires *int64      `json:"auth_exp"`
	NodeDetails *NodeDetail `json:"node_details"`
}

type NodeDetail struct {
	Latitude          *float64      `json:"latitude,omitempty"`
	Longitude         *float64      `json:"longitude,omitempty"`
	LastContact       *int64        `json:"last_contact"`
	InSync            bool          `json:"in_sync"`
	PendingInstall    int           `json:"pending_install"`
	PendingRevocation int           `json:"pending_revocation"`
	Adapters          []string      `json:"adapters"`
	Links             []string      `json:"links"`
	Visas             []int64       `json:"visas"`
	VisaRequests      int           `json:"visa_requests"`
	ApprovedRequests  int           `json:"approved_vreqs"`
	DeniedRequests    int           `json:"denied_vreqs"`
	BufferedDenials   *uint64       `json:"buffered_denials"`
	LocalDenials      *uint64       `json:"local_denials"`
	DenialStatsError  string        `json:"denial_stats_error,omitempty"`
	Counters          []NodeCounter `json:"counters"`
	CounterStatsError string        `json:"counter_stats_error,omitempty"`
	CountersUpdatedAt *time.Time    `json:"counters_updated_at,omitempty"`
}

type NodeCounter struct {
	Group string `json:"group"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Link struct {
	NodeA      string `json:"node_a_addr"`
	NodeB      string `json:"node_b_addr"`
	State      string `json:"ctype"`
	SubstrateA string `json:"node_a_substrate"`
	SubstrateB string `json:"node_b_substrate"`
	LinkID     string `json:"link_id"`
	Cost       int    `json:"link_cost"`
}

type ServiceEntry struct {
	ID string `json:"id"`
}

type Service struct {
	Name                      string `json:"service_name"`
	ActorCN                   string `json:"actor_cn"`
	Address                   string `json:"zpr_addr"`
	Kind                      string `json:"service_kind"`
	Endpoints                 string `json:"service_endpoints"`
	ExternalNetworkConnection string `json:"external_network_connection,omitempty"`
}

type TrustedSource struct {
	Name          string  `json:"name"`
	ActorCN       string  `json:"actor_cn"`
	Provider      string  `json:"provider"`
	ZPRAddress    string  `json:"zpr_addr,omitempty"`
	Endpoints     string  `json:"service_endpoints,omitempty"`
	Health        string  `json:"health"`
	HealthNote    string  `json:"health_note"`
	LastLookupMS  *uint64 `json:"last_lookup_ms"`
	LastSuccessMS *uint64 `json:"last_success_ms"`
	EditorURL     string  `json:"editor_url,omitempty"`
}

type TrustedStatus struct {
	Name          string  `json:"name"`
	Health        string  `json:"health"`
	LastLookupMS  *uint64 `json:"last_lookup_ms"`
	LastSuccessMS *uint64 `json:"last_success_ms"`
}

type VisaEntry struct {
	ID int64 `json:"id"`
}

type Visa struct {
	ID              int64    `json:"id"`
	Created         int64    `json:"created"`
	Expires         int64    `json:"expires"`
	Source          string   `json:"source_addr"`
	Destination     string   `json:"dest_addr"`
	SourcePort      *int     `json:"source_port"`
	DestinationPort *int     `json:"dest_port"`
	Protocol        string   `json:"proto"`
	Direction       string   `json:"direction"`
	RequestingNode  string   `json:"requesting_node"`
	Path            []string `json:"path"`
	PolicyID        string   `json:"policy_id"`
}

type Deny struct {
	Source      string `json:"source_addr"`
	Destination string `json:"dest_addr"`
	Protocol    uint8  `json:"protocol"`
	Port        uint16 `json:"dest_port"`
	Count       uint64 `json:"count"`
	LastDenyMS  uint64 `json:"last_deny_ms"`
	Code        string `json:"deny_code"`
}

type StatsResponse struct {
	Stats map[string]string `json:"stats"`
}
