package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-ldap/ldap/v3"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func evaluationEnvironment(state string, inventory evaluationInventory, mode string) []string {
	config := inventory.Config
	env := []string{}
	// Never inherit service/provider/Simulator configuration from the caller.
	for _, name := range []string{"PATH", "HOME", "TMPDIR", "SYSTEMROOT"} {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	file := func(name string) string { return filepath.Join(state, name) }
	if mode == "control-service" {
		env = append(env,
			"ZPR_TRUSTED_SOURCE_ORGANIZATION_ID=evaluation-"+inventory.InstallID,
			"ZPR_ASSERTION_SOURCES_FILE="+file("assertion-sources.json"),
			"ZPR_ADAPTER_LOG_CONFIG_FILE="+file("adapter-logs.json"),
			"ZPR_CONTROL_SERVICE_LISTEN="+fmt.Sprintf("127.0.0.1:%d", config.ServicePort),
			"ZPR_CONTROL_SERVICE_CERT_FILE="+file("server-chain.pem"),
			"ZPR_CONTROL_SERVICE_KEY_FILE="+file("server-key.pem"),
			"ZPR_CONTROL_SERVICE_CLIENT_CA_FILE="+file("ca.pem"))
		if config.VisaService {
			env = append(env,
				"ZPR_ADMIN_URL="+fmt.Sprintf("https://localhost:%d", config.VSAdminPort),
				"ZPR_ADMIN_CA_FILE="+file("ca.pem"),
				"ZPR_ADMIN_KEY_FILE="+file("vs-admin-api.key"))
		}
		if config.subjectLinks() {
			env = append(env, evaluationSubjectEnvironment(state, config, false)...)
		}
		return env
	}
	if config.subjectLinks() {
		env = append(env, evaluationSubjectEnvironment(state, config, true)...)
	}
	if config.BrowserIP != "" {
		env = append(env, "ZPR_CONTROL_ROOM_DIRECT_IP="+config.BrowserIP)
	}
	return append(env,
		"ZPR_CONTROL_ROOM_LOG_PERMISSION=logs.read",
		"ZPR_OPERATOR_OIDC_CONFIG_FILE="+file("operator-oidc.json"),
		"ZPR_OPERATOR_OIDC_SECRET_FILE="+file("oidc-room-client.secret"),
		"ZPR_OPERATOR_OIDC_CA_FILE="+file("ca.pem"),
		"ZPR_CONTROL_ROOM_ORGANIZATION_ID=evaluation-"+inventory.InstallID,
		"ZPR_CONTROL_ROOM_CERT_FILE="+file("server-chain.pem"),
		"ZPR_CONTROL_ROOM_KEY_FILE="+file("server-key.pem"),
		"ZPR_CONTROL_ROOM_ORIGIN="+config.browserOrigin(config.RoomPort),
		"ZPR_CONTROL_SERVICE_URL="+fmt.Sprintf("https://localhost:%d", config.ServicePort),
		"ZPR_CONTROL_SERVICE_CA_FILE="+file("ca.pem"),
		"ZPR_CONTROL_CLIENT_CERT_FILE="+file("client-chain.pem"),
		"ZPR_CONTROL_CLIENT_KEY_FILE="+file("client-key.pem"))
}

var errEvaluationProcessExited = errors.New("operator process exited")

type evaluationProcess struct {
	command *exec.Cmd
	done    chan error
	log     *os.File
}

func startEvaluation(state string, inventory evaluationInventory, runtime evaluationRuntimeOptions, out io.Writer) error {
	lock, err := lockEvaluation(state)
	if err != nil {
		return err
	}
	defer lock.Close()
	if _, err := readEvaluation(state, inventory.Config); err != nil {
		return err
	}
	runtime, err = runtime.resolve(inventory.Config)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	compose := func(commandContext context.Context, args ...string) error {
		command := exec.CommandContext(commandContext, "docker", append([]string{"compose", "-p", "zpr-evaluation-" + inventory.InstallID, "-f", filepath.Join(state, "compose.json")}, args...)...)
		command.Dir = state
		command.Env = evaluationDockerEnvironment()
		// Compose/Docker errors can interpolate env_file secrets. Never echo them.
		command.Stdout, command.Stderr = io.Discard, io.Discard
		return command.Run()
	}
	preflight, stopPreflight := context.WithTimeout(ctx, 30*time.Second)
	defer stopPreflight()
	if err := verifyEvaluationDockerHost(preflight); err != nil {
		return err
	}
	if err := compose(preflight, "version"); err != nil {
		return errors.New("Docker with Compose v2 is required; render-only does not require Docker")
	}
	for _, image := range []struct{ reference, platform string }{
		{inventory.Config.LDAPImage, "linux/amd64"}, {inventory.Config.LogImage, inventory.Config.LogPlatform}, {inventory.Config.DexImage, inventory.Config.DexPlatform},
	} {
		command := exec.CommandContext(preflight, "docker", "image", "inspect", image.reference, "--format", "{{.Architecture}}")
		command.Env = evaluationDockerEnvironment()
		data, err := command.Output()
		if err != nil || strings.TrimSpace(string(data)) != strings.TrimPrefix(image.platform, "linux/") {
			return fmt.Errorf("required local image %s (%s) unavailable or platform mismatch; load the documented immutable package, then retry (no automatic pulls)", image.reference, image.platform)
		}
	}
	// Reserve all ports until just before startup, preventing contact with an existing stack.
	listeners := []net.Listener{}
	for _, address := range inventory.Config.listenAddresses() {
		listener, err := net.Listen("tcp4", address)
		if err != nil {
			for _, listener := range listeners {
				listener.Close()
			}
			return fmt.Errorf("evaluation listener %s unavailable; check local interface/port availability and the original launch flags", address)
		}
		listeners = append(listeners, listener)
	}
	closePorts := func() {
		for _, listener := range listeners {
			listener.Close()
		}
	}
	defer closePorts()
	fmt.Fprintln(out, "Building current production Control Room/Control-Service package (Simulator assets excluded)…")
	binary := filepath.Join(state, "build", "zpr-control-room")
	build := exec.CommandContext(ctx, "go", "build", "-trimpath", "-tags", "production", "-o", binary, "./cmd/zpr-web-dashboard")
	build.Stdout, build.Stderr = io.Discard, io.Discard
	if err := build.Run(); err != nil {
		return errors.New("production package build failed; run packaging/control-room/build.sh separately for build diagnostics")
	}
	closePorts()
	processes := []*evaluationProcess{}
	// Always remove only this random project's containers/network. Never delete data volumes.
	defer func() {
		for _, process := range processes {
			_ = process.command.Process.Signal(syscall.SIGTERM)
		}
		for _, process := range processes {
			select {
			case <-process.done:
			case <-time.After(5 * time.Second):
				_ = process.command.Process.Kill()
				<-process.done
			}
			process.log.Close()
		}
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if inventory.Config.DataPlane {
			if err := removeEvaluationDataPlane(cleanup, inventory); err != nil {
				fmt.Fprintf(out, "Cleanup incomplete: run docker rm -f %[1]s; docker network rm %[1]s. Private persistent state is retained.\n", evaluationDataPlaneName(inventory))
			}
		}
		if err := compose(cleanup, "down", "--timeout", "5"); err != nil {
			fmt.Fprintf(out, "Cleanup incomplete: from the state directory run docker compose -p zpr-evaluation-%s -f compose.json down. Private persistent state is retained.\n", inventory.InstallID)
		}
	}()
	startup, stopStartup := context.WithTimeout(ctx, 120*time.Second)
	defer stopStartup()
	if err := compose(startup, "up", "-d", "--pull", "never"); err != nil {
		return errors.New("evaluation containers did not start; check Docker availability/image compatibility/port conflicts; private state retained")
	}
	tlsConfig, err := evaluationTLS(state, false)
	if err != nil {
		return err
	}
	if err := waitEvaluation(startup, "starter LDAP", out, func() error { return checkEvaluationLDAP(inventory.Config, state, tlsConfig) }); err != nil {
		return err
	}
	if err := waitEvaluation(startup, "OpenObserve (TLS, admin authentication)", out, func() error { return checkEvaluationLogging(inventory.Config.LogPort, state, tlsConfig) }); err != nil {
		return err
	}
	if err := waitEvaluation(startup, "named-operator OIDC (verified token identity)", out, func() error { return configureEvaluationOperator(startup, state, inventory, tlsConfig) }); err != nil {
		return err
	}
	launch := func(name, executable string, args, env []string) (*evaluationProcess, error) {
		logFile, err := os.OpenFile(filepath.Join(state, "logs", name+".log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return nil, errors.New("cannot open private service log")
		}
		command := exec.Command(executable, args...)
		command.Env = env
		command.Dir = state
		command.Stdout, command.Stderr = logFile, logFile
		if err := command.Start(); err != nil {
			logFile.Close()
			return nil, fmt.Errorf("cannot start %s; inspect private service log", name)
		}
		process := &evaluationProcess{command: command, done: make(chan error, 1), log: logFile}
		processes = append(processes, process)
		go func() { process.done <- command.Wait(); close(process.done) }()
		return process, nil
	}
	exited := func(process *evaluationProcess) error {
		select {
		case <-process.done:
			return errEvaluationProcessExited
		default:
			return nil
		}
	}
	if inventory.Config.DataPlane {
		if err := createEvaluationDataPlaneNetwork(startup, inventory); err != nil {
			return err
		}
		_, markerErr := os.Stat(filepath.Join(state, evaluationVisaPolicyMarker))
		plane, err := launch("data-plane", "docker", evaluationDataPlaneArguments(state, inventory, runtime.DataPlaneImage, errors.Is(markerErr, os.ErrNotExist)), evaluationDockerEnvironment())
		if err != nil {
			return err
		}
		if err := waitEvaluation(startup, "data plane: Visa Service admin API with node and vs.zpr admitted", out, func() error {
			if err := exited(plane); err != nil {
				return err
			}
			if err := checkEvaluationVisa(state, inventory.Config.VSAdminPort, tlsConfig); err != nil {
				return err
			}
			return checkEvaluationDataPlaneActors(state, inventory.Config.VSAdminPort, tlsConfig)
		}); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(state, evaluationVisaPolicyMarker), []byte("per-install data-plane policy installed\n"), 0o600); err != nil {
			return errors.New("cannot record Visa Service policy installation")
		}
	} else if inventory.Config.VisaService {
		valkey, err := launch("valkey", runtime.ValkeyServer, []string{filepath.Join(state, "valkey.conf")}, evaluationVisaEnvironment())
		if err != nil {
			return err
		}
		if err := waitEvaluation(startup, "private Valkey (authenticated)", out, func() error {
			if err := exited(valkey); err != nil {
				return err
			}
			return checkEvaluationValkey(state, inventory.Config.ValkeyPort)
		}); err != nil {
			return err
		}
		visa, err := launch("visa-service", runtime.VisaServiceBinary, evaluationVisaArguments(state), evaluationVisaEnvironment())
		if err != nil {
			return err
		}
		if err := waitEvaluation(startup, "Visa Service admin API (verified TLS, API key)", out, func() error {
			if err := exited(visa); err != nil {
				return err
			}
			return checkEvaluationVisa(state, inventory.Config.VSAdminPort, tlsConfig)
		}); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(state, evaluationVisaPolicyMarker), []byte("bundled evaluation policy installed\n"), 0o600); err != nil {
			return errors.New("cannot record Visa Service policy installation")
		}
	}
	for _, mode := range []string{"control-service", "control-room"} {
		args := []string{"-mode", mode}
		if mode == "control-room" {
			args = append(args, "-listen", inventory.Config.roomListen())
		}
		process, err := launch(mode, binary, args, evaluationEnvironment(state, inventory, mode))
		if err != nil {
			return err
		}
		port, path, mutual := inventory.Config.RoomPort, "/", false
		if mode == "control-service" {
			port, path, mutual = inventory.Config.ServicePort, "/api/trusted-sources/change-feeds", true
		}
		serviceTLS, err := evaluationTLS(state, mutual)
		if err != nil {
			return err
		}
		if err := waitEvaluation(startup, mode, out, func() error {
			if err := exited(process); err != nil {
				return err
			}
			if mode == "control-room" {
				serviceTLS = serviceTLS.Clone()
				serviceTLS.ServerName = inventory.Config.browserHost()
				return checkEvaluationURL(inventory.Config.browserOrigin(port)+path, serviceTLS, "")
			}
			return checkEvaluationHTTP(port, path, serviceTLS, "")
		}); err != nil {
			return err
		}
		if mode == "control-service" && inventory.Config.subjectLinks() {
			if err := waitEvaluation(startup, "subject snapshot delivery (pinned VS client mTLS)", out, func() error {
				if err := exited(process); err != nil {
					return err
				}
				return checkEvaluationDelivery(state, inventory.Config.SnapshotDeliveryPort)
			}); err != nil {
				return err
			}
		}
	}
	var aliasDone <-chan error
	if inventory.Config.BrowserIP != "" {
		closeAlias, done, err := startEvaluationLocalAlias(state, inventory.Config)
		if err != nil {
			return err
		}
		defer closeAlias()
		aliasDone = done
	}
	if err := registerEvaluationSource(startup, state, inventory); err != nil {
		return err
	}
	if err := waitEvaluation(startup, "evaluation lifecycle ingestion/query", out, func() error { return collectEvaluationLog(startup, state, inventory, "bootstrap_ready") }); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s\nREADY (B1-B3 evaluation): %s\nOpenObserve administrator: admin@evaluation.invalid at https://localhost:%d (password: private logging-admin.secret).\nTrust ONLY this install's ca.pem in your evaluation browser; never disable certificate checks.\n", evaluationBanner, inventory.Config.browserOrigin(inventory.Config.RoomPort), inventory.Config.LogPort)
	if inventory.Config.BrowserIP != "" {
		fmt.Fprintf(out, "Local entrypoint https://localhost:%d redirects to the canonical network origin. Restrict browser/OIDC ports to intended clients; other services remain loopback-only.\n", inventory.Config.RoomPort)
	}
	upstream := "Upstream Visa/Policy services are NOT configured. Control Room reports unavailable dependencies explicitly."
	if inventory.Config.VisaService {
		upstream = fmt.Sprintf("Loopback Visa Service admin at https://localhost:%d (bundled evaluation policy; no nodes or enforcement). Policy-Service is NOT configured.", inventory.Config.VSAdminPort)
		if inventory.Config.DataPlane {
			endpoints := "no endpoints"
			if inventory.Config.DataPlaneEndpoints > 0 {
				endpoints = fmt.Sprintf("endpoint-01..%02d admitted by bootstrap key (X15); run them with the Simulator via SIMULATOR_DATA_PLANE_ENDPOINTS_FILE=%s", inventory.Config.DataPlaneEndpoints, filepath.Join(state, evaluationDataPlaneEndpointsFile))
			}
			upstream = fmt.Sprintf("Data-plane Visa Service admin at https://localhost:%d (container %s: one node and vs.zpr admitted; %s). Policy-Service is NOT configured.", inventory.Config.VSAdminPort, evaluationDataPlaneName(inventory), endpoints)
		}
	}
	fmt.Fprintln(out, "Operator login: admin@evaluation.invalid (password: private operator-admin.secret). Administrator-scoped evaluation grants; no shared sessions.\nStarter directory registered; lifecycle events ingested into evaluation logging. "+upstream)
	if inventory.Config.subjectLinks() {
		initCommand := evaluationVSInitCommand(runtime.VisaServiceBinary, state)
		if inventory.Config.DataPlane {
			initCommand = evaluationDataPlaneInitCommand(inventory, state)
		}
		fmt.Fprintf(out, "Subject links: independent reviewer %s (password: private operator-reviewer.secret); see %s for the publication cycle.\nAfter the first signed snapshot, initialize the VS checkpoint once: %s\n",
			evaluationReviewerEmail, filepath.Join(state, "SUBJECT-LINKS.txt"), initCommand)
	}
	fmt.Fprintln(out, "Foreground supervision active. Ctrl-C stops this install's services/containers; rerun the SAME command to preserve identity and directory/log data.")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(out, "Stopping evaluation services; private state retained.")
			return nil
		case <-aliasDone:
			return errors.New("localhost HTTPS entrypoint exited; stopping this install; inspect private state and retry")
		case <-ticker.C:
			if err := collectEvaluationLog(ctx, state, inventory, "bootstrap_heartbeat"); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
			for _, process := range processes {
				select {
				case <-process.done:
					return errors.New("operator service exited; inspect private service logs and rerun the same command")
				default:
				}
			}
			if checkEvaluationHTTP(inventory.Config.OIDCPort, "/.well-known/openid-configuration", tlsConfig, "") != nil || checkEvaluationLDAP(inventory.Config, state, tlsConfig) != nil || checkEvaluationLogging(inventory.Config.LogPort, state, tlsConfig) != nil {
				return errors.New("starter LDAP/logging lost readiness; stopping only this install; inspect private state and retry")
			}
		}
	}
}

func waitEvaluation(ctx context.Context, name string, out io.Writer, check func() error) error {
	fmt.Fprintf(out, "WAIT %s…\n", name)
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%s not ready before timeout/cancellation; inspect private state/logs and retry the same command", name)
		}
		checkErr := check()
		if errors.Is(checkErr, errEvaluationProcessExited) {
			return fmt.Errorf("%s exited; inspect its private service log and retry", name)
		}
		if checkErr == nil {
			fmt.Fprintf(out, "HEALTHY %s\n", name)
			return nil
		}
		select {
		case <-ctx.Done():
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func evaluationTLS(state string, mutual bool) (*tls.Config, error) {
	ca, err := os.ReadFile(filepath.Join(state, "ca.pem"))
	if err != nil {
		return nil, errors.New("cannot read install CA")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, errors.New("invalid install CA")
	}
	config := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool, ServerName: "localhost"}
	if mutual {
		certificate, err := tls.LoadX509KeyPair(filepath.Join(state, "client-chain.pem"), filepath.Join(state, "client-key.pem"))
		if err != nil {
			return nil, errors.New("invalid Control Room client identity")
		}
		config.Certificates = []tls.Certificate{certificate}
	}
	return config, nil
}

func checkEvaluationLDAP(evaluation evaluationConfig, state string, config *tls.Config) error {
	port := evaluation.LDAPPort
	socket, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", fmt.Sprintf("127.0.0.1:%d", port), config)
	if err != nil {
		return errors.New("LDAPS handshake failed")
	}
	connection := ldap.NewConn(socket, true)
	connection.Start()
	defer connection.Close()
	connection.SetTimeout(2 * time.Second)
	secret, err := os.ReadFile(filepath.Join(state, "ldap-reader.secret"))
	if err != nil {
		return errors.New("cannot read LDAP health credential")
	}
	if err := connection.Bind(evaluation.directoryReaderDN(), strings.TrimSpace(string(secret))); err != nil {
		return errors.New("LDAP health bind failed")
	}
	_, err = connection.Search(ldap.NewSearchRequest(evaluation.directoryBaseDN(), ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 2, false, "(objectClass=*)", []string{"dn"}, nil))
	if err != nil {
		return errors.New("LDAP base search failed")
	}
	return nil
}

func checkEvaluationLogging(port int, state string, config *tls.Config) error {
	secret, err := os.ReadFile(filepath.Join(state, "logging-admin.secret"))
	if err != nil {
		return errors.New("cannot read logging health credential")
	}
	if checkEvaluationHTTP(port, "/api/default/streams", config, "") == nil {
		return errors.New("OpenObserve allows unauthenticated queries; refuse public logging")
	}
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin@evaluation.invalid:"+strings.TrimSpace(string(secret))))
	return checkEvaluationHTTP(port, "/api/default/streams", config, auth)
}

func checkEvaluationHTTP(port int, path string, config *tls.Config, authorization string) error {
	return checkEvaluationURL(fmt.Sprintf("https://localhost:%d%s", port, path), config, authorization)
}

func checkEvaluationURL(address string, config *tls.Config, authorization string) error {
	transport := &http.Transport{TLSClientConfig: config, Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 3 * time.Second, Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequest(http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("verified HTTPS readiness failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("HTTPS readiness returned non-success")
	}
	return nil
}

// Obtain and verify the IdP's actual subject; never invent a grant from an email.
func configureEvaluationOperator(ctx context.Context, state string, inventory evaluationInventory, tlsConfig *tls.Config) error {
	tlsConfig = tlsConfig.Clone()
	tlsConfig.ServerName = inventory.Config.browserHost()
	transport := &http.Transport{TLSClientConfig: tlsConfig, Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 3 * time.Second, Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	verifiedContext := oidc.ClientContext(ctx, client)
	issuer := inventory.Config.browserOrigin(inventory.Config.OIDCPort)
	provider, err := oidc.NewProvider(verifiedContext, issuer)
	if err != nil {
		return errors.New("OIDC discovery not ready")
	}
	var metadata struct {
		Keys string `json:"jwks_uri"`
	}
	if provider.Claims(&metadata) != nil || !evaluationOIDCEndpoints(issuer, provider.Endpoint().AuthURL, provider.Endpoint().TokenURL, metadata.Keys) {
		return errors.New("OIDC discovery points outside this install; refuse credential disclosure")
	}
	secret, err := os.ReadFile(filepath.Join(state, "oidc-room-client.secret"))
	if err != nil {
		return errors.New("cannot read private OIDC client secret")
	}
	subject := func(username, secretFile string) (string, error) {
		password, err := os.ReadFile(filepath.Join(state, secretFile))
		if err != nil {
			return "", errors.New("cannot read private bootstrap operator credential")
		}
		form := url.Values{"grant_type": {"password"}, "scope": {"openid profile email"}, "username": {username}, "password": {strings.TrimSpace(string(password))}}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.Endpoint().TokenURL, strings.NewReader(form.Encode()))
		if err != nil {
			return "", errors.New("invalid OIDC token endpoint")
		}
		request.SetBasicAuth("zpr-control-room", strings.TrimSpace(string(secret)))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response, err := client.Do(request)
		if err != nil {
			return "", errors.New("bootstrap operator authentication not ready")
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return "", errors.New("bootstrap operator authentication rejected")
		}
		var token struct {
			IDToken string `json:"id_token"`
		}
		if json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&token) != nil || token.IDToken == "" {
			return "", errors.New("OIDC returned no identity token")
		}
		identity, err := provider.Verifier(&oidc.Config{ClientID: "zpr-control-room"}).Verify(verifiedContext, token.IDToken)
		if err != nil || identity.Subject == "" {
			return "", errors.New("OIDC identity verification failed")
		}
		return identity.Subject, nil
	}
	adminSubject, err := subject("admin@evaluation.invalid", "operator-admin.secret")
	if err != nil {
		return err
	}
	organization := evaluationOrganization(inventory)
	admin := operatorGrantIdentity{Subject: adminSubject, Permissions: []string{"monitor.read", "logs.read", "policy.read", "policy.edit", "policy.analyze"}}
	var reviewer operatorGrantIdentity
	if inventory.Config.subjectLinks() {
		admin.Permissions = append(admin.Permissions, evaluationAdminSubjectLinkPermissions...)
		reviewerSubject, err := subject(evaluationReviewerEmail, "operator-reviewer.secret")
		if err != nil {
			return err
		}
		if reviewerSubject == adminSubject {
			return errors.New("evaluation reviewer must be an independent OIDC subject")
		}
		reviewer = operatorGrantIdentity{Subject: reviewerSubject, Permissions: evaluationReviewerPermissions}
	}
	config := operatorauth.Config{Version: 1, Issuer: issuer, ClientID: "zpr-control-room", RedirectURL: inventory.Config.browserOrigin(inventory.Config.RoomPort) + "/auth/operator/callback", SessionLifetimeSeconds: 3600,
		Grants: []operatorauth.Grant{{Issuer: issuer, Subject: admin.Subject, Organizations: []string{organization}, Permissions: admin.Permissions}}}
	if reviewer.Subject != "" {
		config.Grants = append(config.Grants, operatorauth.Grant{Issuer: issuer, Subject: reviewer.Subject, Organizations: []string{organization}, Permissions: reviewer.Permissions})
	}
	if err := config.Validate(); err != nil {
		return errors.New("verified bootstrap operator grant is invalid")
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	if err := writeEvaluationOnce(state, "operator-oidc.json", append(data, '\n')); err != nil {
		return err
	}
	if reviewer.Subject == "" {
		return nil
	}
	signerKey, err := os.ReadFile(filepath.Join(state, "operator-delegation-key.pem"))
	if err != nil {
		return errors.New("cannot read private operator delegation key")
	}
	clientChain, err := os.ReadFile(filepath.Join(state, "client-chain.pem"))
	if err != nil {
		return errors.New("cannot read Control Room client certificate")
	}
	trust, err := evaluationDelegationTrustFile(inventory.Config, issuer, organization, signerKey, clientChain, admin, reviewer)
	if err != nil {
		return err
	}
	var parsed operatordelegation.Config
	if json.Unmarshal(trust, &parsed) != nil || parsed.Validate() != nil {
		return errors.New("verified operator delegation trust is invalid")
	}
	return writeEvaluationOnce(state, evaluationDelegationTrust, trust)
}

// writeEvaluationOnce stores a verified identity binding once and refuses any
// later difference rather than silently rebinding grants.
func writeEvaluationOnce(state, name string, data []byte) error {
	existing, err := os.ReadFile(filepath.Join(state, name))
	if err == nil {
		if string(existing) != string(data) {
			return errors.New("stored operator identity changed; refuse implicit rebind")
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot read existing operator grant")
	}
	root, err := os.OpenRoot(state)
	if err != nil {
		return err
	}
	defer root.Close()
	return writePrivate(root, name, data)
}

func evaluationDockerEnvironment() []string {
	env := []string{}
	for _, name := range []string{"PATH", "HOME", "TMPDIR", "DOCKER_CONFIG", "DOCKER_CONTEXT", "DOCKER_HOST"} {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
}

func verifyEvaluationDockerHost(ctx context.Context) error {
	endpoint := os.Getenv("DOCKER_HOST")
	// DOCKER_CONTEXT overrides DOCKER_HOST in Docker CLI semantics.
	if endpoint == "" || os.Getenv("DOCKER_CONTEXT") != "" {
		command := exec.CommandContext(ctx, "docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}")
		command.Env = evaluationDockerEnvironment()
		data, err := command.Output()
		if err != nil {
			return errors.New("cannot inspect local Docker context; install/configure Docker or use render-only")
		}
		endpoint = strings.TrimSpace(string(data))
	}
	if !strings.HasPrefix(endpoint, "unix:///") {
		return errors.New("evaluation bootstrap requires a local Unix-socket Docker daemon; remote/TCP contexts are not supported")
	}
	return nil
}

func evaluationOIDCEndpoints(issuer, authorization, token, keys string) bool {
	return authorization == issuer+"/auth" && token == issuer+"/token" && keys == issuer+"/keys"
}
