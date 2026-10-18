package systemconfig

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
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
	ListenAddress  string             `json:"listen_address"`
	Environment    map[string]string  `json:"environment"`
	OIDCConfigPath string             `json:"operator_oidc_config_path"`
	OIDCConfigMode uint32             `json:"operator_oidc_config_mode"`
	OIDCConfig     json.RawMessage    `json:"operator_oidc_config"`
	FileMounts     []RuntimeFileMount `json:"file_mounts"`
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
	if config.OIDC.CAFile != "" {
		environment["ZPR_OPERATOR_OIDC_CA_FILE"] = config.OIDC.CAFile
	}
	if config.ControlService.TLSServerName != "" {
		environment["ZPR_CONTROL_SERVICE_TLS_SERVER_NAME"] = config.ControlService.TLSServerName
	}
	oidcConfig, err := json.MarshalIndent(config.OIDC.Config, "", "  ")
	if err != nil {
		return ControlRoomRuntime{}, errors.New("cannot encode operator OIDC runtime configuration")
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
		paths = append(paths, config.OIDC.CAFile)
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
	}, nil
}
