package systemconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"

	"neboagency.com/zpr-dashborad/internal/servicehttp"

	"gopkg.in/yaml.v3"
)

const (
	maxRuntimeOIDCConfigBytes = 64 << 10
	maxControlRoomComposeSize = 256 << 10
)

var controlRoomImageReference = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:@-]{0,254}$`)

// RenderOperatorOIDCConfig writes only the runtime OIDC settings, never the client secret.
// Existing identical protected output is accepted to make retries safe; other output is preserved.
func (config Config) RenderOperatorOIDCConfig(outputPath string) error {
	if err := config.Validate(); err != nil {
		return err
	}
	if !path.IsAbs(outputPath) || path.Clean(outputPath) != outputPath {
		return errors.New("operator OIDC output must be a canonical absolute file path")
	}

	data, err := json.MarshalIndent(config.OIDC.Config, "", "  ")
	if err != nil {
		return errors.New("cannot encode operator OIDC runtime configuration")
	}
	data = append(data, '\n')
	if len(data) > maxRuntimeOIDCConfigBytes {
		return errors.New("operator OIDC runtime configuration exceeds 64 KiB")
	}
	return writeProtectedOutput(outputPath, data, maxRuntimeOIDCConfigBytes, "operator OIDC runtime configuration")
}

// RenderControlRoomCompose writes a minimal host-network service descriptor for
// the existing Control Room binary. It does not build an image or start a service.
func (config Config) RenderControlRoomCompose(outputPath, image, operatorOIDCPath string) error {
	if err := config.Validate(); err != nil {
		return err
	}
	if config.ControlRoom.ManagementIP == "" {
		return errors.New("Control Room Compose requires an explicitly selected private management IP")
	}
	if !path.IsAbs(outputPath) || path.Clean(outputPath) != outputPath || outputPath == "/" {
		return errors.New("Control Room Compose output must be a canonical absolute file path")
	}
	if !path.IsAbs(operatorOIDCPath) || path.Clean(operatorOIDCPath) != operatorOIDCPath ||
		operatorOIDCPath == "/" || operatorOIDCPath == outputPath {
		return errors.New("operator OIDC runtime config must be a distinct canonical absolute file path")
	}
	operatorOIDCData, err := json.MarshalIndent(config.OIDC.Config, "", "  ")
	if err != nil {
		return errors.New("cannot encode operator OIDC runtime configuration")
	}
	operatorOIDCData = append(operatorOIDCData, '\n')
	if err := verifyExistingProtectedOutput(operatorOIDCPath, operatorOIDCData, maxRuntimeOIDCConfigBytes, "operator OIDC runtime configuration"); err != nil {
		return errors.New("Control Room Compose requires the identical protected operator OIDC runtime config to be rendered first")
	}
	if !controlRoomImageReference.MatchString(image) {
		return errors.New("Control Room image must be an explicit local image reference without whitespace")
	}
	uid, gid := os.Getuid(), os.Getgid()
	if uid <= 0 || gid < 0 {
		return errors.New("Control Room Compose must be rendered by its non-root runtime owner")
	}
	for _, inputPath := range []string{
		config.ControlRoom.CertificateFile, config.ControlRoom.KeyFile,
		config.ControlService.ClientCertificateFile, config.ControlService.ClientKeyFile,
		config.ControlService.ServerCAFile, config.OIDC.ClientSecretFile,
		config.OIDC.CAFile, operatorOIDCPath,
	} {
		if inputPath != "" && inputPath == outputPath {
			return errors.New("Control Room Compose output must not replace a runtime input file")
		}
	}

	origin, err := url.Parse(config.ControlRoom.Origin)
	if err != nil {
		return errors.New("Control Room origin is invalid")
	}
	port := origin.Port()
	if port == "" {
		port = "443"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return errors.New("Control Room origin port is invalid")
	}

	const (
		roomCertPath       = "/run/zpr/control-room.crt"
		roomKeyPath        = "/run/zpr/control-room.key"
		operatorOIDCPathIn = "/run/zpr/operator-oidc.json"
		operatorSecretPath = "/run/zpr/operator-oidc-secret"
		operatorCAPath     = "/run/zpr/operator-oidc-ca.pem"
		serviceCertPath    = "/run/zpr/control-service-client.crt"
		serviceKeyPath     = "/run/zpr/control-service-client.key"
		serviceCAPath      = "/run/zpr/control-service-ca.pem"
	)
	environment := map[string]string{
		"ZPR_CONTROL_ROOM_ORIGIN":          config.ControlRoom.Origin,
		"ZPR_CONTROL_ROOM_MANAGEMENT_IP":   config.ControlRoom.ManagementIP,
		"ZPR_CONTROL_ROOM_CERT_FILE":       roomCertPath,
		"ZPR_CONTROL_ROOM_KEY_FILE":        roomKeyPath,
		"ZPR_CONTROL_ROOM_ORGANIZATION_ID": config.Organization.ID,
		"ZPR_OPERATOR_OIDC_CONFIG_FILE":    operatorOIDCPathIn,
		"ZPR_OPERATOR_OIDC_SECRET_FILE":    operatorSecretPath,
		"ZPR_CONTROL_SERVICE_URL":          config.ControlService.URL,
		"ZPR_CONTROL_CLIENT_CERT_FILE":     serviceCertPath,
		"ZPR_CONTROL_CLIENT_KEY_FILE":      serviceKeyPath,
		"ZPR_CONTROL_SERVICE_CA_FILE":      serviceCAPath,
	}
	volumes := []controlRoomComposeVolume{
		{Type: "bind", Source: config.ControlRoom.CertificateFile, Target: roomCertPath, ReadOnly: true},
		{Type: "bind", Source: config.ControlRoom.KeyFile, Target: roomKeyPath, ReadOnly: true},
		{Type: "bind", Source: operatorOIDCPath, Target: operatorOIDCPathIn, ReadOnly: true},
		{Type: "bind", Source: config.OIDC.ClientSecretFile, Target: operatorSecretPath, ReadOnly: true},
		{Type: "bind", Source: config.ControlService.ClientCertificateFile, Target: serviceCertPath, ReadOnly: true},
		{Type: "bind", Source: config.ControlService.ClientKeyFile, Target: serviceKeyPath, ReadOnly: true},
		{Type: "bind", Source: config.ControlService.ServerCAFile, Target: serviceCAPath, ReadOnly: true},
	}
	if config.OIDC.CAFile != "" {
		environment["ZPR_OPERATOR_OIDC_CA_FILE"] = operatorCAPath
		volumes = append(volumes, controlRoomComposeVolume{
			Type: "bind", Source: config.OIDC.CAFile, Target: operatorCAPath, ReadOnly: true,
		})
	}
	compose := controlRoomCompose{
		Services: map[string]controlRoomComposeService{
			"control-room": {
				Image: image, PullPolicy: "never", Platform: "linux/amd64", User: fmt.Sprintf("%d:%d", uid, gid),
				NetworkMode: "host", Restart: "unless-stopped", Init: true, ReadOnly: true,
				CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges:true"},
				Command:     []string{"-mode", "control-room", "-listen", net.JoinHostPort(config.ControlRoom.ManagementIP, port)},
				Environment: environment, Volumes: volumes,
			},
		},
	}
	data, err := yaml.Marshal(compose)
	if err != nil {
		return errors.New("cannot encode Control Room Compose definition")
	}
	if len(data) > maxControlRoomComposeSize {
		return errors.New("Control Room Compose definition exceeds 256 KiB")
	}
	return writeProtectedOutput(outputPath, data, maxControlRoomComposeSize, "Control Room Compose definition")
}

type controlRoomCompose struct {
	Services map[string]controlRoomComposeService `yaml:"services"`
}

type controlRoomComposeService struct {
	Image       string                     `yaml:"image"`
	PullPolicy  string                     `yaml:"pull_policy"`
	Platform    string                     `yaml:"platform"`
	User        string                     `yaml:"user"`
	NetworkMode string                     `yaml:"network_mode"`
	Restart     string                     `yaml:"restart"`
	Init        bool                       `yaml:"init"`
	ReadOnly    bool                       `yaml:"read_only"`
	CapDrop     []string                   `yaml:"cap_drop"`
	SecurityOpt []string                   `yaml:"security_opt"`
	Command     []string                   `yaml:"command"`
	Environment map[string]string          `yaml:"environment"`
	Volumes     []controlRoomComposeVolume `yaml:"volumes"`
}

type controlRoomComposeVolume struct {
	Type     string `yaml:"type"`
	Source   string `yaml:"source"`
	Target   string `yaml:"target"`
	ReadOnly bool   `yaml:"read_only"`
}

func writeProtectedOutput(outputPath string, data []byte, limit int64, description string) error {
	file, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return verifyExistingProtectedOutput(outputPath, data, limit, description)
	}
	if err != nil {
		return fmt.Errorf("cannot create %s", description)
	}

	if err := file.Chmod(0o600); err != nil {
		return removeIncompleteProtectedOutput(file, outputPath, true, description)
	}
	written, err := file.Write(data)
	if err != nil || written != len(data) {
		return removeIncompleteProtectedOutput(file, outputPath, true, description)
	}
	if err := file.Sync(); err != nil {
		return removeIncompleteProtectedOutput(file, outputPath, true, description)
	}
	if err := file.Close(); err != nil {
		return removeIncompleteProtectedOutput(file, outputPath, false, description)
	}
	return nil
}

func verifyExistingProtectedOutput(outputPath string, expected []byte, limit int64, description string) error {
	actual, err := servicehttp.ReadOperatorFile(outputPath, limit, true)
	if err != nil {
		return fmt.Errorf("existing %s is not a protected regular file", description)
	}
	if !bytes.Equal(actual, expected) {
		return fmt.Errorf("existing %s differs; refusing to overwrite", description)
	}
	return nil
}

func removeIncompleteProtectedOutput(file *os.File, outputPath string, closeFile bool, description string) error {
	closeErr := error(nil)
	if closeFile {
		closeErr = file.Close()
	}
	removeErr := os.Remove(outputPath)
	if closeErr != nil || (removeErr != nil && !errors.Is(removeErr, os.ErrNotExist)) {
		return fmt.Errorf("%s failed and incomplete output cleanup failed", description)
	}
	return fmt.Errorf("cannot write %s", description)
}
