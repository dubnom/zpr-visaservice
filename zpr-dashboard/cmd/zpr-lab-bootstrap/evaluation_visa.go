package main

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// evaluationVisaPolicy is integration-test/pregen/zpt-test.bin2: a test-signed
// policy compiled from zpt-test.zpl. It configures no enforcement here because
// the evaluation install has no nodes or adapters.
//
//go:embed evaluation-visa-policy.bin2
var evaluationVisaPolicy []byte

var evaluationVisaFiles = []string{"vs-policy.bin2", "vs.toml", "vs-admin-keys.toml", "vs-admin-api.key", "valkey.conf", "valkey.secret"}

// evaluationVisaRequired lists protected VS files; the data plane compiles its
// own per-install policy instead of the bundled one.
func evaluationVisaRequired(config evaluationConfig) []string {
	if !config.DataPlane {
		return evaluationVisaFiles
	}
	return append(slices.DeleteFunc(slices.Clone(evaluationVisaFiles), func(name string) bool { return name == "vs-policy.bin2" }), evaluationDataPlaneRequired(config)...)
}

// evaluationVisaPolicyMarker is mutable state: the bundled policy is installed
// into Valkey once; later starts keep any policy applied through Control Room.
const evaluationVisaPolicyMarker = "vs-policy.installed"

func generateEvaluationVisa(files map[string][]byte, state string, inventory evaluationInventory, now time.Time) error {
	if strings.ContainsAny(state, "\"\\\x00\r\n") {
		return errors.New("-visa-service state path must not contain quotes, backslashes or control characters")
	}
	config := inventory.Config
	keyID, secret, valkey := make([]byte, 4), make([]byte, 32), make([]byte, 32)
	for _, buffer := range [][]byte{keyID, secret, valkey} {
		if _, err := rand.Read(buffer); err != nil {
			return err
		}
	}
	id := hex.EncodeToString(keyID)
	hash := sha256.Sum256(secret)
	password := hex.EncodeToString(valkey)
	quoted := func(name string) string {
		data, _ := json.Marshal(filepath.Join(state, name))
		return string(data)
	}
	valkeyPort, valkeyBind := config.ValkeyPort, "127.0.0.1"
	if config.DataPlane {
		if err := evaluationDataPlaneMountsSafe(state); err != nil {
			return err
		}
		if err := generateEvaluationDataPlane(files, state, inventory, now); err != nil {
			return err
		}
		valkeyPort = 6379
	} else {
		files["vs-policy.bin2"] = slices.Clone(evaluationVisaPolicy)
	}
	files["valkey.secret"] = []byte(password + "\n")
	files["vs-admin-api.key"] = []byte("zpr_vsapi." + id + "." + base64.RawURLEncoding.EncodeToString(secret) + "\n")
	files["vs-admin-keys.toml"] = fmt.Appendf(nil, `[keys.%s]
owner = "evaluation-control-service"
permission = "readwrite"
status = "active"
created = %q
secret_hash = %q
description = "EVALUATION ONLY Control-Service administration"
`, id, now.Format(time.RFC3339), hex.EncodeToString(hash[:]))
	files["valkey.conf"] = fmt.Appendf(nil, `bind %s
port %d
protected-mode yes
requirepass %s
dir %s
appendonly yes
save ""
daemonize no
logfile ""
`, valkeyBind, valkeyPort, password, quoted("valkey-state"))
	if config.DataPlane {
		// vs_addr/vsapi_port keep their ZPR defaults (fd5a:5052::1, 5002) inside
		// the data plane; admin is relayed and published only to host loopback.
		files["vs.toml"] = fmt.Appendf(nil, `# EVALUATION ONLY data-plane Visa Service: one node and the vs.zpr adapter.
[core]
admin_addr = "10.0.0.2"
admin_port = %d
admin_cert = %s
admin_key = %s
vk_uri = "redis://:%s@127.0.0.1:%d"
identity = "evaluation-%s"
api_keys = %s
file_ts_dir = %s
`, evaluationDataPlaneAdminPort, quoted("server-chain.pem"), quoted("server-key.pem"), password, valkeyPort, inventory.InstallID, quoted("vs-admin-keys.toml"), quoted("vs-trusted-services"))
		coordinate := func(value float64) string {
			text := strconv.FormatFloat(value, 'f', -1, 64)
			if !strings.Contains(text, ".") {
				text += ".0"
			}
			return text
		}
		if location := inventory.Config.NodeLocation; location != nil {
			files["vs.toml"] = fmt.Appendf(files["vs.toml"], "\n# Operator-configured map position of the data-plane node.\n[nodes.\"node\"]\nlatitude = %s\nlongitude = %s\n", coordinate(location.Latitude), coordinate(location.Longitude))
		}
		for _, node := range inventory.Config.DataPlaneNodes {
			files["vs.toml"] = fmt.Appendf(files["vs.toml"], "\n# Operator-configured map position of an additional node.\n[nodes.%q]\nlatitude = %s\nlongitude = %s\n", node.Name, coordinate(node.Location.Latitude), coordinate(node.Location.Longitude))
		}
		return nil
	}
	files["vs.toml"] = fmt.Appendf(nil, `# EVALUATION ONLY loopback Visa Service. No nodes, adapters or enforcement.
[core]
vs_addr = "127.0.0.1"
admin_addr = "127.0.0.1"
vsapi_port = %d
admin_port = %d
admin_cert = %s
admin_key = %s
vk_uri = "redis://:%s@127.0.0.1:%d"
identity = "evaluation-%s"
api_keys = %s
file_ts_dir = %s
`, config.VSAPIPort, config.VSAdminPort, quoted("server-chain.pem"), quoted("server-key.pem"), password, config.ValkeyPort, inventory.InstallID, quoted("vs-admin-keys.toml"), quoted("vs-trusted-services"))
	return nil
}

// resolve checks executables before any port is reserved or container started.
func (options evaluationRuntimeOptions) resolve(config evaluationConfig) (evaluationRuntimeOptions, error) {
	if !config.VisaService {
		return options, nil
	}
	if config.DataPlane {
		if options.DataPlaneImage == "" || strings.ContainsAny(options.DataPlaneImage, " \t\n") {
			return options, errors.New("-data-plane-image must name a local image")
		}
		if err := exec.Command("docker", "image", "inspect", "--format", "{{.Id}}", options.DataPlaneImage).Run(); err != nil {
			return options, fmt.Errorf("data-plane image %s not found; build it with packaging/evaluation/data-plane/build.sh", options.DataPlaneImage)
		}
		return options, nil
	}
	if !filepath.IsAbs(options.VisaServiceBinary) {
		return options, errors.New("-visa-service requires -vs-binary with the absolute path of a built vs executable")
	}
	info, err := os.Stat(options.VisaServiceBinary)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return options, errors.New("-vs-binary must be an existing executable file")
	}
	valkey, err := exec.LookPath(options.ValkeyServer)
	if err != nil {
		return options, errors.New("valkey-server not found; install Valkey or pass -valkey-server")
	}
	options.ValkeyServer = valkey
	return options, nil
}

func evaluationVisaEnvironment() []string {
	env := []string{}
	// Like operator services, never inherit caller configuration.
	for _, name := range []string{"PATH", "HOME", "TMPDIR"} {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
}

func evaluationVisaArguments(state string) []string {
	args := []string{"-c", filepath.Join(state, "vs.toml")}
	if _, err := os.Stat(filepath.Join(state, evaluationVisaPolicyMarker)); errors.Is(err, os.ErrNotExist) {
		args = append(args, filepath.Join(state, "vs-policy.bin2"))
	}
	return args
}

func checkEvaluationValkey(state string, port int) error {
	password, err := os.ReadFile(filepath.Join(state, "valkey.secret"))
	if err != nil {
		return errors.New("cannot read private Valkey credential")
	}
	connection, err := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		return errors.New("Valkey not listening")
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := fmt.Fprintf(connection, "AUTH %s\r\nPING\r\n", strings.TrimSpace(string(password))); err != nil {
		return errors.New("Valkey write failed")
	}
	reader := bufio.NewReader(connection)
	for _, expected := range []string{"+OK\r\n", "+PONG\r\n"} {
		line, err := reader.ReadString('\n')
		if err != nil || line != expected {
			return errors.New("Valkey authentication/ping failed")
		}
	}
	return nil
}

func checkEvaluationVisa(state string, port int, config *tls.Config) error {
	key, err := os.ReadFile(filepath.Join(state, "vs-admin-api.key"))
	if err != nil {
		return errors.New("cannot read private Visa Service admin key")
	}
	transport := &http.Transport{TLSClientConfig: config, Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 3 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://localhost:%d/admin/policies", port), nil)
	if err != nil {
		return err
	}
	request.Header.Set("X-API-Key", strings.TrimSpace(string(key)))
	response, err := client.Do(request)
	if err != nil {
		return errors.New("Visa Service admin HTTPS not ready")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 65536))
	if response.StatusCode != http.StatusOK {
		return errors.New("Visa Service admin API rejected the evaluation key")
	}
	return nil
}
