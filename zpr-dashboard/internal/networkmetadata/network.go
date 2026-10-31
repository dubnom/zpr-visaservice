// Package networkmetadata defines presentation metadata, never a trust identity.
package networkmetadata

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Metadata struct {
	Name string `json:"name"`
}

// Validate allows an unnamed network and deliberately imposes no uniqueness rule.
func (metadata Metadata) Validate() error {
	if !utf8.ValidString(metadata.Name) || len(metadata.Name) > 256 ||
		strings.TrimSpace(metadata.Name) != metadata.Name {
		return errors.New("network name must be valid UTF-8, at most 256 bytes, without surrounding whitespace")
	}
	for _, character := range metadata.Name {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return errors.New("network name must not contain control or formatting characters")
		}
	}
	return nil
}
