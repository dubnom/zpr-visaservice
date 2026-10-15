package systemconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

const MaxConfigBytes = 64 << 10

// Config is the management bootstrap input, not a complete runtime configuration.
type Config struct {
	SchemaVersion int          `json:"schema_version"`
	Organization  Organization `json:"organization"`
	ControlRoom   ControlRoom  `json:"control_room"`
	OIDC          OIDC         `json:"oidc"`
}

type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ControlRoom struct {
	Origin          string `json:"origin"`
	ManagementIP    string `json:"management_ip,omitempty"`
	CertificateFile string `json:"certificate_file"`
	KeyFile         string `json:"key_file"`
}

type OIDC struct {
	Config           operatorauth.Config `json:"config"`
	ClientSecretFile string              `json:"client_secret_file"`
	CAFile           string              `json:"ca_file,omitempty"`
}

var organizationID = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)

// Read validates shape and references only. It does not open referenced files,
// contact an identity provider, or change any service.
func Read(reader io.Reader) (Config, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxConfigBytes+1))
	if err != nil {
		return Config{}, errors.New("unable to read company configuration")
	}
	if len(data) > MaxConfigBytes {
		return Config{}, errors.New("company configuration exceeds 64 KiB")
	}
	shape := json.NewDecoder(bytes.NewReader(data))
	if err := checkJSONValue(shape, 0); err != nil {
		return Config{}, err
	}
	if _, err := shape.Token(); err != io.EOF {
		return Config{}, errors.New("company configuration must contain exactly one JSON document")
	}
	var config Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, errors.New("company configuration has unknown fields or invalid field types")
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func checkJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 16 {
		return errors.New("company configuration nesting exceeds 16 levels")
	}
	token, err := decoder.Token()
	if err != nil {
		return errors.New("company configuration is not valid JSON")
	}
	if token == nil {
		return errors.New("company configuration must not contain null values")
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return errors.New("company configuration is not valid JSON")
			}
			name, ok := key.(string)
			if !ok || name != strings.ToLower(name) {
				return errors.New("company configuration field names must be lowercase")
			}
			if seen[name] {
				return errors.New("company configuration has duplicate fields")
			}
			seen[name] = true
			if err := checkJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := checkJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("company configuration is not valid JSON")
	}
	if _, err := decoder.Token(); err != nil {
		return errors.New("company configuration is not valid JSON")
	}
	return nil
}

func (config Config) Validate() error {
	if config.SchemaVersion != 1 {
		return errors.New("company configuration requires schema_version 1")
	}
	if !organizationID.MatchString(config.Organization.ID) || !plainText(config.Organization.Name, 256) {
		return errors.New("organization requires a 1-64 character lowercase alphanumeric/hyphen ID and a nonempty name")
	}
	if err := ValidateManagementEndpoint(config.ControlRoom.Origin, config.ControlRoom.ManagementIP); err != nil {
		return err
	}
	if err := ValidateOperatorAccess(config.Organization.ID, config.ControlRoom.Origin, config.OIDC.Config); err != nil {
		return err
	}
	files := []string{config.ControlRoom.CertificateFile, config.ControlRoom.KeyFile, config.OIDC.ClientSecretFile}
	if config.OIDC.CAFile != "" {
		files = append(files, config.OIDC.CAFile)
	}
	seen := map[string]bool{}
	for _, file := range files {
		if !plainText(file, 4096) || !path.IsAbs(file) || path.Clean(file) != file ||
			file == "/" || strings.ContainsAny(file, `\$`) || seen[file] {
			return errors.New("TLS/OIDC file references must be distinct canonical absolute Linux file paths without expansion")
		}
		seen[file] = true
	}
	return nil
}

// ValidateManagementEndpoint checks explicit production input, not network reachability.
// Empty managementIP leaves binding unselected for structure-only bootstrap input.
func ValidateManagementEndpoint(origin, managementIP string) error {
	address, err := url.Parse(origin)
	if err != nil || !httpsAddress(address) || address.Path != "" ||
		origin != "https://"+address.Host || !companyHostname(address.Hostname()) {
		return errors.New("control_room.origin must be an exact HTTPS origin with a lowercase company DNS hostname and optional valid port")
	}
	if managementIP != "" {
		ip := net.ParseIP(managementIP)
		if ip == nil || !ip.IsPrivate() || ip.String() != managementIP {
			return errors.New("control_room.management_ip must be a canonical private IPv4 or IPv6 address")
		}
	}
	return nil
}

// ValidateOperatorAccess keeps production bootstrap and runtime grant scopes aligned.
func ValidateOperatorAccess(organization, origin string, config operatorauth.Config) error {
	if !organizationID.MatchString(organization) {
		return errors.New("Control Room requires an explicit company organization ID")
	}
	if err := config.Validate(); err != nil {
		return errors.New("oidc.config does not satisfy the operator OIDC configuration contract")
	}
	issuer, err := url.Parse(config.Issuer)
	if err != nil || !httpsAddress(issuer) || strings.Contains(config.Issuer, "#") {
		return errors.New("oidc.config.issuer must use HTTPS without credentials, query or fragment")
	}
	if config.RedirectURL != origin+"/auth/operator/callback" {
		return errors.New("OIDC redirect_url must exactly match control_room.origin plus /auth/operator/callback")
	}
	monitoringGrant := false
	for _, grant := range config.Grants {
		if len(grant.Organizations) != 1 || grant.Organizations[0] != organization {
			return errors.New("each operator grant must scope exactly the configured organization")
		}
		for _, permission := range grant.Permissions {
			if !slices.Contains([]string{
				"read", "create", "cancel", "approve", "reject",
				"monitor.read", "policy.read", "policy.analyze", "policy.edit",
				"gateway.read", "gateway.analyze", "gateway.edit",
				"change.read", "change.submit", "change.review",
			}, permission) {
				return errors.New("company grants must use production Control Room or enrollment permissions")
			}
			monitoringGrant = monitoringGrant || permission == "monitor.read"
		}
	}
	if !monitoringGrant {
		return errors.New("at least one explicitly named operator must have monitor.read for the configured organization")
	}
	return nil
}

func httpsAddress(address *url.URL) bool {
	if address == nil || address.Scheme != "https" || address.Hostname() == "" ||
		address.User != nil || address.Opaque != "" || address.RawQuery != "" ||
		address.ForceQuery || address.Fragment != "" ||
		strings.HasSuffix(address.Host, ":") {
		return false
	}
	if port := address.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 || strconv.Itoa(number) != port {
			return false
		}
	}
	return true
}

func companyHostname(host string) bool {
	if len(host) > 253 || !strings.Contains(host, ".") || net.ParseIP(host) != nil ||
		strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-') {
				return false
			}
		}
	}
	return true
}

func plainText(value string, limit int) bool {
	if value == "" || len(value) > limit || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < 32 || character == 127 {
			return false
		}
	}
	return true
}
