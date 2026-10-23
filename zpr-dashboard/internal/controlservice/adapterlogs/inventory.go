package adapterlogs

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Inventory struct {
	Adapters []Target `json:"adapters"`
}

type Target struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Sources []SourceConfig `json:"sources"`
}

type SourceConfig struct {
	URL              string `json:"url,omitempty"`
	CAFile           string `json:"ca_file,omitempty"`
	PasswordFile     string `json:"password_file,omitempty"`
	Username         string `json:"username,omitempty"`
	Organization     string `json:"organization,omitempty"`
	Stream           string `json:"stream,omitempty"`
	Name             string `json:"name"`
	Kind             string `json:"kind"`
	File             string `json:"file,omitempty"`
	Container        string `json:"container,omitempty"`
	Path             string `json:"path,omitempty"`
	RuntimeContainer string `json:"runtime_container,omitempty"`
}

var streamIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

var containerName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// ReadInventory reads only the explicitly supplied operator configuration file.
func ReadInventory(path string) (Inventory, error) {
	var inventory Inventory
	file, err := os.Open(path)
	if err != nil {
		return inventory, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 65537))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&inventory); err != nil {
		return inventory, err
	}
	if len(inventory.Adapters) > 100 {
		return inventory, errors.New("too many adapter log targets")
	}
	seen := make(map[string]bool)
	for _, adapter := range inventory.Adapters {
		if adapter.ID == "" || adapter.Name == "" || seen[adapter.ID] || len(adapter.Sources) > 8 {
			return inventory, errors.New("invalid adapter log target")
		}
		seen[adapter.ID] = true
		for _, source := range adapter.Sources {
			if source.RuntimeContainer != "" && !containerName.MatchString(source.RuntimeContainer) {
				return inventory, errors.New("invalid runtime container")
			}
			if source.Name == "" || source.Kind != "adapter" && source.Kind != "controller" {
				return inventory, errors.New("invalid adapter log source")
			}
			if source.URL != "" {
				endpoint, err := url.Parse(source.URL)
				if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.ForceQuery || strings.Contains(source.URL, "#") || source.Username == "" || !filepath.IsAbs(source.PasswordFile) || !streamIdentifier.MatchString(source.Organization) || !streamIdentifier.MatchString(source.Stream) || source.File != "" || source.Container != "" || source.Path != "" || source.RuntimeContainer != "" {
					return inventory, errors.New("invalid OpenObserve log source")
				}
			} else if source.PasswordFile != "" || source.Username != "" || source.Organization != "" || source.Stream != "" || source.CAFile != "" {
				return inventory, errors.New("network log fields require an HTTPS source")
			} else if source.File != "" {
				if !filepath.IsAbs(source.File) || source.Container != "" || source.Path != "" {
					return inventory, errors.New("invalid file log source")
				}
			} else if !containerName.MatchString(source.Container) || source.Path != "" && !filepath.IsAbs(source.Path) {
				return inventory, errors.New("invalid container log source")
			}
		}
	}
	return inventory, nil
}
