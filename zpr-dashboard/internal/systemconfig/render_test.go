//go:build linux || darwin

package systemconfig

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func TestRenderOperatorOIDCConfigIsProtectedAndIdempotent(t *testing.T) {
	config := validConfig()
	output := filepath.Join(t.TempDir(), "operator-oidc.json")

	if err := config.RenderOperatorOIDCConfig(output); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
	if bytes.Contains(data, []byte("private-secret-value")) || bytes.Contains(data, []byte(config.OIDC.ClientSecretFile)) {
		t.Fatal("rendered OIDC runtime config includes secret material or its source path")
	}
	var rendered operatorauth.Config
	if err := json.Unmarshal(data, &rendered); err != nil {
		t.Fatalf("rendered output is not a runtime OIDC config: %v", err)
	}
	if !reflect.DeepEqual(rendered, config.OIDC.Config) {
		t.Fatal("rendered OIDC config differs from the validated company input")
	}
	if err := rendered.Validate(); err != nil {
		t.Fatalf("runtime OIDC config rejects rendered output: %v", err)
	}
	if err := config.RenderOperatorOIDCConfig(output); err != nil {
		t.Fatalf("identical output was not safely reusable: %v", err)
	}

	changed := config
	changed.OIDC.Config.ClientID = "different-client"
	if err := changed.RenderOperatorOIDCConfig(output); err == nil {
		t.Fatal("different output overwrote existing runtime config")
	}
	after, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, data) {
		t.Fatal("refused overwrite changed existing runtime config")
	}
}

func TestRenderOperatorOIDCConfigRejectsUnsafeOutputPaths(t *testing.T) {
	config := validConfig()
	for _, output := range []string{"relative.json", "/tmp/../operator-oidc.json"} {
		t.Run(output, func(t *testing.T) {
			if err := config.RenderOperatorOIDCConfig(output); err == nil {
				t.Fatal("noncanonical output path was accepted")
			}
		})
	}

	directory := t.TempDir()
	target := filepath.Join(directory, "target.json")
	if err := os.WriteFile(target, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := config.RenderOperatorOIDCConfig(link); err == nil {
		t.Fatal("symlink output was accepted")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "preserve" {
		t.Fatal("symlink target was modified")
	}
}

func TestRenderControlRoomComposeRequiresHostIPAndProtectedOIDCOutput(t *testing.T) {
	config := validConfig()
	directory := t.TempDir()
	oidcPath := filepath.Join(directory, "operator-oidc.json")
	composePath := filepath.Join(directory, "compose.yaml")

	if err := config.RenderControlRoomCompose(composePath, "zpr-control-room:engineering", oidcPath); err == nil {
		t.Fatal("host-network Compose accepted an unselected management IP")
	}
	config.ControlRoom.ManagementIP = "10.20.30.40"
	if err := config.RenderControlRoomCompose(composePath, "zpr-control-room:engineering", oidcPath); err == nil {
		t.Fatal("Compose accepted a missing protected OIDC runtime config")
	}
	if _, err := os.Lstat(composePath); !os.IsNotExist(err) {
		t.Fatal("failed Compose rendering created output")
	}
}
