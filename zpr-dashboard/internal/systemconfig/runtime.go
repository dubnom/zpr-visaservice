package systemconfig

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"path"
	"slices"
)

const operatorOIDCRuntimeConfigPath = "/etc/zpr/config/operator-oidc.json"
const operatorOIDCRuntimeConfigMode = 0o600

type RuntimeFileMount struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	ReadOnly    bool   `json:"read_only"`
}

type ControlRoomRuntime struct {
	ListenAddress  string                 `json:"listen_address"`
	Environment    map[string]string      `json:"environment"`
	OIDCConfigPath string                 `json:"operator_oidc_config_path"`
	OIDCConfigMode uint32                 `json:"operator_oidc_config_mode"`
	OIDCConfig     json.RawMessage        `json:"operator_oidc_config"`
	FileMounts     []RuntimeFileMount     `json:"file_mounts"`
	ControlService *ControlServiceRuntime `json:"control_service,omitempty"`
}

type ControlServiceRuntime struct {
	ListenAddress string             `json:"listen_address"`
	Environment   map[string]string  `json:"environment"`
	FileMounts    []RuntimeFileMount `json:"file_mounts"`
}

// RenderControlRoomRuntime maps validated company inputs to the existing
// Control Room process contract. It does not inspect files or start services.
func RenderControlRoomRuntime(config Config) (ControlRoomRuntime, error) {
	if err := config.Validate(); err != nil {
		return ControlRoomRuntime{}, err
	}
	if config.SchemaVersion != 2 || config.ControlService == nil || config.ControlRoom.ManagementIP == "" {
		return ControlRoomRuntime{}, errors.New("Control Room runtime rendering requires schema_version 2 and an explicit management IP")
	}
	origin, err := url.Parse(config.ControlRoom.Origin)
	if err != nil {
		return ControlRoomRuntime{}, errors.New("Control Room origin is invalid")
	}
	port := origin.Port()
	if port == "" {
		port = "443"
	}
	environment := map[string]string{
		"ZPR_CONTROL_ROOM_CERT_FILE":       config.ControlRoom.CertificateFile,
		"ZPR_CONTROL_ROOM_KEY_FILE":        config.ControlRoom.KeyFile,
		"ZPR_CONTROL_ROOM_ORIGIN":          config.ControlRoom.Origin,
		"ZPR_CONTROL_ROOM_MANAGEMENT_IP":   config.ControlRoom.ManagementIP,
		"ZPR_CONTROL_ROOM_ORGANIZATION_ID": config.Organization.ID,
		"ZPR_OPERATOR_OIDC_CONFIG_FILE":    operatorOIDCRuntimeConfigPath,
		"ZPR_OPERATOR_OIDC_SECRET_FILE":    config.OIDC.ClientSecretFile,
		"ZPR_CONTROL_SERVICE_URL":          config.ControlService.URL,
		"ZPR_CONTROL_CLIENT_CERT_FILE":     config.ControlService.ClientCertificateFile,
		"ZPR_CONTROL_CLIENT_KEY_FILE":      config.ControlService.ClientKeyFile,
		"ZPR_CONTROL_SERVICE_CA_FILE":      config.ControlService.CAFile,
	}
	paths := []string{
		config.ControlRoom.CertificateFile,
		config.ControlRoom.KeyFile,
		config.OIDC.ClientSecretFile,
		config.ControlService.ClientCertificateFile,
		config.ControlService.ClientKeyFile,
		config.ControlService.CAFile,
	}
	if config.OIDC.CAFile != "" {
		environment["ZPR_OPERATOR_OIDC_CA_FILE"] = config.OIDC.CAFile
		paths = append(paths, config.OIDC.CAFile)
	}
	if config.ControlService.TLSServerName != "" {
		environment["ZPR_CONTROL_SERVICE_TLS_SERVER_NAME"] = config.ControlService.TLSServerName
	}
	var authorityRuntime *ControlServiceRuntime
	if config.BudgetAuthority != nil {
		authority := config.BudgetAuthority
		environment["ZPR_OPERATOR_DELEGATION_SIGNER_FILE"] = authority.ControlRoomSignerFile
		environment["ZPR_OPERATOR_DELEGATION_KEY_FILE"] = authority.ControlRoomSignerKeyFile
		environment["ZPR_OPERATOR_DELEGATION_AUDIENCE"] = authority.ControlRoomSignerAudience

		authorityRuntime = &ControlServiceRuntime{
			ListenAddress: authority.ServiceListenAddress,
			Environment: map[string]string{
				"ZPR_CONTROL_SERVICE_LISTEN":         authority.ServiceListenAddress,
				"ZPR_CONTROL_SERVICE_CERT_FILE":      authority.ServiceCertificateFile,
				"ZPR_CONTROL_SERVICE_KEY_FILE":       authority.ServiceKeyFile,
				"ZPR_CONTROL_SERVICE_CLIENT_CA_FILE": authority.ServiceClientCAFile,
				"ZPR_BUDGET_AUTHORITY_DB_PATH":       authority.DatabaseFile,
				"ZPR_AUDIT_DB_PATH":                  path.Join(path.Dir(authority.DatabaseFile), "audit.db"),
				"ZPR_OPERATOR_DELEGATION_TRUST_FILE": authority.DelegationTrustFile,
			},
			FileMounts: []RuntimeFileMount{
				{Source: path.Dir(authority.DatabaseFile), Destination: path.Dir(authority.DatabaseFile), ReadOnly: false},
				{Source: authority.DelegationTrustFile, Destination: authority.DelegationTrustFile, ReadOnly: true},
				{Source: authority.ServiceCertificateFile, Destination: authority.ServiceCertificateFile, ReadOnly: true},
				{Source: authority.ServiceKeyFile, Destination: authority.ServiceKeyFile, ReadOnly: true},
				{Source: authority.ServiceClientCAFile, Destination: authority.ServiceClientCAFile, ReadOnly: true},
			},
		}
		if config.NetworkName != "" {
			authorityRuntime.Environment["ZPR_NETWORK_NAME"] = config.NetworkName
		}
		paths = append(paths, authority.ControlRoomSignerFile, authority.ControlRoomSignerKeyFile)
	}
	oidcConfig, err := json.MarshalIndent(config.OIDC.Config, "", "  ")
	if err != nil {
		return ControlRoomRuntime{}, errors.New("cannot encode operator OIDC runtime configuration")
	}
	slices.Sort(paths)
	mounts := make([]RuntimeFileMount, 0, len(paths))
	for _, file := range paths {
		mounts = append(mounts, RuntimeFileMount{Source: file, Destination: file, ReadOnly: true})
	}
	return ControlRoomRuntime{
		ListenAddress:  net.JoinHostPort(config.ControlRoom.ManagementIP, port),
		Environment:    environment,
		OIDCConfigPath: operatorOIDCRuntimeConfigPath,
		OIDCConfigMode: operatorOIDCRuntimeConfigMode,
		OIDCConfig:     oidcConfig,
		FileMounts:     mounts,
		ControlService: authorityRuntime,
	}, nil
}
