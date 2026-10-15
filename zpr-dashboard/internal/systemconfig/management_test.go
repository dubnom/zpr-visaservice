package systemconfig

import "testing"

func TestCompanyManagementIP(t *testing.T) {
	for _, test := range []struct {
		ip    string
		valid bool
	}{
		{"", true}, {"10.20.30.40", true}, {"172.16.0.1", true}, {"192.168.1.10", true}, {"fd12:3456::1", true},
		{"127.0.0.1", false}, {"::1", false}, {"0.0.0.0", false}, {"::", false}, {"8.8.8.8", false},
		{"169.254.1.2", false}, {"fe80::1", false}, {"224.0.0.1", false}, {"room.example.com", false},
		{" 10.20.30.40", false}, {"10.20.30.40:443", false}, {"010.20.30.40", false},
		{"::ffff:10.20.30.40", false}, {"FD12:3456::1", false}, {"10.20.30.0/24", false},
	} {
		t.Run(test.ip, func(t *testing.T) {
			config := validConfig()
			config.ControlRoom.ManagementIP = test.ip
			if err := config.Validate(); (err == nil) != test.valid {
				t.Fatalf("management IP valid=%t err=%v", test.valid, err)
			}
		})
	}
}
