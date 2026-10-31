package setupmode

import "testing"

func TestParse(t *testing.T) {
	for _, test := range []struct {
		value string
		want  Mode
		valid bool
	}{
		{"", LegacyLauncher, true},
		{"legacy-launcher", LegacyLauncher, true},
		{"ordinary-client", OrdinaryClient, true},
		{" ordinary-client ", Invalid, false},
		{"other", Invalid, false},
	} {
		t.Run(test.value, func(t *testing.T) {
			got, err := Parse(test.value)
			if (err == nil) != test.valid || got != test.want {
				t.Fatalf("Parse(%q) = %v, %v", test.value, got, err)
			}
		})
	}
}
