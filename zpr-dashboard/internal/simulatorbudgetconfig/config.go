package simulatorbudgetconfig

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"path"
	"regexp"
	"strings"

	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

const MaxConfigBytes = 64 << 10

type Config struct {
	SchemaVersion         int    `json:"schema_version"`
	URL                   string `json:"url"`
	ClientCertificateFile string `json:"client_certificate_file"`
	ClientKeyFile         string `json:"client_key_file"`
	CAFile                string `json:"ca_file"`
	TLSServerName         string `json:"tls_server_name,omitempty"`
}

type FileMount struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	ReadOnly    bool   `json:"read_only"`
}

type Runtime struct {
	Environment map[string]string `json:"environment"`
	FileMounts  []FileMount       `json:"file_mounts"`
}

var dnsName = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*\.?$`)

func Read(reader io.Reader) (Config, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxConfigBytes+1))
	if err != nil {
		return Config{}, errors.New("unable to read Simulator budget client configuration")
	}
	if len(data) > MaxConfigBytes {
		return Config{}, errors.New("Simulator budget client configuration exceeds 64 KiB")
	}
	shape := json.NewDecoder(bytes.NewReader(data))
	if err := checkJSONValue(shape, 0); err != nil {
		return Config{}, err
	}
	if _, err := shape.Token(); err != io.EOF {
		return Config{}, errors.New("Simulator budget client configuration must contain exactly one JSON document")
	}
	var config Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, errors.New("Simulator budget client configuration has unknown fields or invalid field types")
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func checkJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 16 {
		return errors.New("Simulator budget client configuration nesting exceeds 16 levels")
	}
	token, err := decoder.Token()
	if err != nil || token == nil {
		return errors.New("Simulator budget client configuration is not valid JSON")
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
				return errors.New("Simulator budget client configuration is not valid JSON")
			}
			name, ok := key.(string)
			if !ok || name != strings.ToLower(name) {
				return errors.New("Simulator budget client configuration field names must be lowercase")
			}
			if seen[name] {
				return errors.New("Simulator budget client configuration has duplicate fields")
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
		return errors.New("Simulator budget client configuration is not valid JSON")
	}
	if _, err := decoder.Token(); err != nil {
		return errors.New("Simulator budget client configuration is not valid JSON")
	}
	return nil
}

func (config Config) Validate() error {
	origin, err := url.Parse(config.URL)
	if config.SchemaVersion != 1 || err != nil || origin.Scheme != "https" || origin.Host == "" ||
		origin.User != nil || (origin.Path != "" && origin.Path != "/") || origin.RawQuery != "" ||
		origin.ForceQuery || origin.Fragment != "" || origin.Opaque != "" ||
		(config.URL != "https://"+origin.Host && config.URL != "https://"+origin.Host+"/") {
		return errors.New("Simulator budget authority URL must be an exact HTTPS origin")
	}
	paths := []string{config.ClientCertificateFile, config.ClientKeyFile, config.CAFile}
	seen := make(map[string]bool, len(paths))
	for _, file := range paths {
		if !canonicalFile(file) || seen[file] {
			return errors.New("Simulator budget client files must be distinct canonical absolute Linux paths")
		}
		seen[file] = true
	}
	if config.TLSServerName != "" && !validServerName(config.TLSServerName) {
		return errors.New("tls_server_name must be a DNS name or IP literal without a port or URL components")
	}
	return nil
}

func (config Config) CheckFiles() error {
	files := []struct {
		name    string
		path    string
		private bool
	}{
		{"client_certificate_file", config.ClientCertificateFile, false},
		{"client_key_file", config.ClientKeyFile, true},
		{"ca_file", config.CAFile, false},
	}
	contents := make([][]byte, len(files))
	defer func() {
		for _, content := range contents {
			clear(content)
		}
	}()
	for index, file := range files {
		data, err := servicehttp.ReadOperatorFile(file.path, 64<<10, file.private)
		if err != nil {
			return fmt.Errorf("%s must reference a protected operator file", file.name)
		}
		contents[index] = data
	}
	if _, err := tls.X509KeyPair(contents[0], contents[1]); err != nil {
		return errors.New("client_certificate_file and client_key_file must contain a matching PEM TLS client identity")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(contents[2]) {
		return errors.New("ca_file must contain at least one PEM certificate")
	}
	return nil
}

func (config Config) Runtime() Runtime {
	environment := map[string]string{
		"ZPR_BUDGET_AUTHORITY_URL":              config.URL,
		"ZPR_BUDGET_AUTHORITY_CLIENT_CERT_FILE": config.ClientCertificateFile,
		"ZPR_BUDGET_AUTHORITY_CLIENT_KEY_FILE":  config.ClientKeyFile,
		"ZPR_BUDGET_AUTHORITY_CA_FILE":          config.CAFile,
	}
	if config.TLSServerName != "" {
		environment["ZPR_BUDGET_AUTHORITY_TLS_SERVER_NAME"] = config.TLSServerName
	}
	return Runtime{
		Environment: environment,
		FileMounts: []FileMount{
			{Source: config.ClientCertificateFile, Destination: config.ClientCertificateFile, ReadOnly: true},
			{Source: config.ClientKeyFile, Destination: config.ClientKeyFile, ReadOnly: true},
			{Source: config.CAFile, Destination: config.CAFile, ReadOnly: true},
		},
	}
}

func canonicalFile(value string) bool {
	return value != "" && len(value) <= 4096 && path.IsAbs(value) && path.Clean(value) == value &&
		value != "/" && !strings.ContainsAny(value, `\$`) && !strings.ContainsAny(value, "\x00\r\n")
}

func validServerName(value string) bool {
	if strings.ContainsAny(value, " /\\:@?#[]\x00\r\n") {
		return false
	}
	if ip := net.ParseIP(value); ip != nil {
		return ip.String() == value
	}
	return len(value) <= 253 && dnsName.MatchString(value)
}
