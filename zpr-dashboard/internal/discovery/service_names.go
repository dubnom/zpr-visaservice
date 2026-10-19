package discovery

import "strings"

// DNS is the familiar name for IANA's "domain"; submissions is the modern
// TCP/465 assignment alongside the older "urd" registry entry.
var serviceHints = func() map[uint16]string {
	names := make(map[uint16]string, len(ianaTCPServices))
	for port, aliases := range ianaTCPServices {
		names[port] = strings.ToLower(strings.Split(aliases, ",")[0])
	}
	names[53] = "dns"
	names[465] = "submissions"
	return names
}()

func serviceDNSLabel(name string) string {
	return strings.Trim(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '-'
	}, strings.ToLower(name)), "-")
}
