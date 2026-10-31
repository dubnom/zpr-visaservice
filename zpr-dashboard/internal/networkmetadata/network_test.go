package networkmetadata

import (
	"strings"
	"testing"
)

func TestDisplayNames(t *testing.T) {
	for _, name := range []string{"", "Research Network", "Research Network", "Réseau ZPR", strings.Repeat("a", 256)} {
		if err := (Metadata{Name: name}).Validate(); err != nil {
			t.Errorf("valid non-unique display name %q rejected: %v", name, err)
		}
	}
	for _, name := range []string{" ", " leading", "trailing ", "line\nbreak", "tab\tname", "spoof\u202ename", "\x7f", string([]byte{0xff}), strings.Repeat("a", 257)} {
		if err := (Metadata{Name: name}).Validate(); err == nil {
			t.Errorf("invalid display name %q accepted", name)
		}
	}
}
