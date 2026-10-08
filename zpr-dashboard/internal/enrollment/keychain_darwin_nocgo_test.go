//go:build darwin && !cgo

package enrollment

import "testing"

func TestMacKeychainWithoutCGOFailsAtStartup(t *testing.T) {
	config := setupConfig(t)
	config.KeyProtection = "macos-keychain"
	if config.Validate() == nil {
		t.Fatal("unavailable Keychain implementation accepted")
	}
	if server, err := NewSetupServer(config); err == nil {
		server.Close()
		t.Fatal("Keychain setup started without native support")
	}
}
