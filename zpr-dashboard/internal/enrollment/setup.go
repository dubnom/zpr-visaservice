//go:build linux || darwin || windows

package enrollment

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/x509"
	"embed"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"
)

//go:embed setup/*
var setupFiles embed.FS

type SetupConfig struct {
	Version                  int    `json:"version"`
	Audience                 string `json:"audience"`
	CAFile                   string `json:"ca_file"`
	StateDirectory           string `json:"state_directory"`
	AllowSoftwareDevelopment bool   `json:"allow_software_development"`
	KeyProtection            string `json:"key_protection,omitempty"`
}

func (c SetupConfig) Validate() error {
	if c.Version != 1 || validateAudience(c.Audience) != nil || !c.AllowSoftwareDevelopment ||
		!filepath.IsAbs(c.StateDirectory) || filepath.Clean(c.StateDirectory) != c.StateDirectory ||
		filepath.Dir(c.StateDirectory) == c.StateDirectory {
		return errors.New("setup requires version 1, a trusted HTTPS audience, an absolute clean state directory, and explicit software-development approval")
	}
	if c.KeyProtection != "" && (c.KeyProtection != "macos-keychain" || runtime.GOOS != "darwin") {
		return errors.New("key_protection must be omitted, or macos-keychain on macOS")
	}
	if c.KeyProtection == "macos-keychain" && !keychainAvailable() {
		return errors.New("macos-keychain requires a native CGO-enabled Security.framework build")
	}
	return nil
}

func LoadSetupConfig(path string) (SetupConfig, error) {
	return loadSetupConfig(path, "")
}

func loadSetupConfig(path, userState string) (SetupConfig, error) {
	data, err := readSetupFile(path)
	if err != nil {
		return SetupConfig{}, fmt.Errorf("read trusted setup configuration: %w", err)
	}
	var config SetupConfig
	if err := decodeJSON(bytes.NewReader(data), &config); err != nil {
		return SetupConfig{}, errors.New("invalid setup configuration JSON")
	}
	if userState != "" {
		if config.StateDirectory != "" {
			return SetupConfig{}, errors.New("desktop configuration must omit state_directory; state belongs to the logged-in user")
		}
		config.StateDirectory = userState
	}
	base, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return SetupConfig{}, err
	}
	if config.CAFile != "" && !filepath.IsAbs(config.CAFile) {
		config.CAFile = filepath.Join(base, config.CAFile)
	}
	if err := config.Validate(); err != nil {
		return SetupConfig{}, err
	}
	return config, nil
}

type SetupServer struct {
	config    SetupConfig
	roots     *x509.CertPool
	listener  net.Listener
	server    *http.Server
	origin    string
	token     string
	expires   time.Time
	mu        sync.Mutex
	client    *Client
	identity  *SoftwareIdentity
	status    *DeviceStatus
	uncertain bool
	checked   bool
	recovery  bool
	next      time.Time
}

type setupView struct {
	Audience           string           `json:"audience"`
	Metadata           *LocalEnrollment `json:"metadata,omitempty"`
	Fingerprint        string           `json:"fingerprint,omitempty"`
	RuntimeFingerprint string           `json:"runtime_fingerprint,omitempty"`
	Status             *DeviceStatus    `json:"status,omitempty"`
	Uncertain          bool             `json:"uncertain"`
	CanClaim           bool             `json:"can_claim"`
	ConfirmRecovery    bool             `json:"confirm_recovery"`
	RetryAfter         int              `json:"retry_after_seconds"`
	Message            string           `json:"message"`
	KeyProtection      string           `json:"key_protection"`
}

func NewSetupServer(config SetupConfig) (*SetupServer, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	var roots *x509.CertPool
	if config.CAFile != "" {
		data, err := readSetupFile(config.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read setup CA: %w", err)
		}
		roots = x509.NewCertPool()
		if !roots.AppendCertsFromPEM(data) {
			return nil, errors.New("setup CA file contains no certificates")
		}
	}
	s := &SetupServer{config: config, roots: roots, token: rand.Text(), expires: time.Now().Add(time.Hour)}
	identity, err := loadSetupIdentity(config)
	if err == nil {
		if err := s.attach(identity); err != nil {
			return nil, err
		}
		s.uncertain = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load setup identity: %w", err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		if s.client != nil {
			s.client.Close()
		}
		return nil, err
	}
	s.listener = listener
	s.origin = "http://" + listener.Addr().String()
	s.server = &http.Server{Handler: http.HandlerFunc(s.serveHTTP),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 40 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	return s, nil
}

func (s *SetupServer) attach(identity *SoftwareIdentity) error {
	if identity.Metadata().Audience != s.config.Audience {
		return errors.New("stored identity audience differs from trusted setup configuration; administrator recovery is required")
	}
	var client *Client
	var err error
	if runtimeSigner := identity.RuntimeSigner(); runtimeSigner != nil {
		client, err = NewClientWithRuntimeKey(s.config.Audience, s.roots, identity, runtimeSigner)
	} else {
		client, err = NewClient(s.config.Audience, s.roots, identity)
	}
	if err != nil {
		return err
	}
	s.identity, s.client = identity, client
	return nil
}

// URL carries a short-lived local capability in a fragment, not an HTTP query.
func (s *SetupServer) URL() string { return s.origin + "/#session=" + s.token }

func (s *SetupServer) Close() error {
	err := s.server.Close()
	listenerErr := s.listener.Close()
	if !errors.Is(listenerErr, net.ErrClosed) {
		err = errors.Join(err, listenerErr)
	}
	if s.client != nil {
		s.client.Close()
	}
	return err
}

func (s *SetupServer) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	done := make(chan struct{})
	shutdown := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			timeout, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			err := s.server.Shutdown(timeout)
			if err != nil {
				err = errors.Join(err, s.server.Close())
			}
			shutdown <- err
		case <-done:
			shutdown <- nil
		}
	}()
	err := s.server.Serve(s.listener)
	close(done)
	shutdownErr := <-shutdown
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return errors.Join(err, shutdownErr)
}

func (s *SetupServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	if r.Host != s.listener.Addr().String() || r.URL.RawQuery != "" ||
		(r.Header.Get("Origin") != "" && r.Header.Get("Origin") != s.origin) ||
		(r.Header.Get("Sec-Fetch-Site") != "" && r.Header.Get("Sec-Fetch-Site") != "same-origin" && r.Header.Get("Sec-Fetch-Site") != "none") {
		apiError(w, http.StatusForbidden, "Invalid local setup origin or host.")
		return
	}
	switch r.URL.Path {
	case "/", "/setup.js", "/setup.css":
		if r.Method != http.MethodGet {
			apiError(w, http.StatusMethodNotAllowed, "GET required.")
			return
		}
		name, content := "index.html", "text/html; charset=utf-8"
		if r.URL.Path == "/setup.js" {
			name, content = "setup.js", "text/javascript; charset=utf-8"
		} else if r.URL.Path == "/setup.css" {
			name, content = "setup.css", "text/css; charset=utf-8"
		}
		data, err := setupFiles.ReadFile("setup/" + name)
		if err != nil {
			apiError(w, http.StatusInternalServerError, "Setup resource unavailable.")
			return
		}
		w.Header().Set("Content-Type", content)
		if _, err := w.Write(data); err != nil {
			log.Printf("Local setup resource delivery failed: %T", err)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if !time.Now().Before(s.expires) ||
		subtle.ConstantTimeCompare([]byte(r.Header.Get("X-ZPR-Setup-Session")), []byte(s.token)) != 1 {
		apiError(w, http.StatusUnauthorized, "Setup session missing or expired. Reopen the complete setup URL, or restart the command for a new session.")
		return
	}
	if r.URL.Path != "/api/session" && r.URL.Path != "/api/prepare" && r.URL.Path != "/api/claim" && r.URL.Path != "/api/status" {
		apiError(w, http.StatusNotFound, "Setup route not found.")
		return
	}
	expected := http.MethodPost
	if r.URL.Path == "/api/session" {
		expected = http.MethodGet
	}
	if r.Method != expected {
		apiError(w, http.StatusMethodNotAllowed, "Invalid setup method.")
		return
	}
	if expected == http.MethodPost && r.Header.Get("Origin") != s.origin {
		apiError(w, http.StatusForbidden, "Same-origin setup request required.")
		return
	}
	if !s.mu.TryLock() {
		w.Header().Set("Retry-After", "2")
		apiError(w, http.StatusConflict, "Another setup operation is running; retry later.")
		return
	}
	defer s.mu.Unlock()
	if r.URL.Path == "/api/session" {
		s.reply(w, http.StatusOK, "Software development key only. Check status before resuming a previous request.")
		return
	}
	var input struct {
		Organization    string `json:"organization"`
		InvitationID    string `json:"invitation_id"`
		Code            string `json:"code"`
		ConfirmRecovery bool   `json:"confirm_recovery"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	if r.Header.Get("Content-Type") != "application/json" || decodeJSON(r.Body, &input) != nil {
		s.reply(w, http.StatusBadRequest, "Invalid setup request.")
		return
	}
	if r.URL.Path == "/api/prepare" {
		if s.identity != nil || input.Code != "" || input.ConfirmRecovery || !validText(input.Organization) || !validText(input.InvitationID) {
			s.reply(w, http.StatusConflict, "Identity already exists or invitation details are invalid; no key was replaced.")
			return
		}
		identity, err := createSetupIdentity(s.config, LocalEnrollment{
			Audience: s.config.Audience, Organization: input.Organization, InvitationID: input.InvitationID})
		if err != nil {
			message := "Could not persist identity. Restart to inspect local state; no claim was sent."
			if s.config.KeyProtection == "macos-keychain" {
				message = "Could not save Keychain identity. Unlock/authorize your default Keychain and restart to inspect state; no claim was sent and no plaintext fallback was used."
			}
			log.Printf("Local setup identity persistence failed: %v", err)
			s.reply(w, http.StatusInternalServerError, message)
			return
		}
		if err := s.attach(identity); err != nil {
			s.reply(w, http.StatusInternalServerError, "Could not initialize identity; restart to inspect local state.")
			return
		}
		s.checked = true
		s.reply(w, http.StatusOK, "Local key saved. Verify these invitation details, then enter the separately supplied code.")
		return
	}
	if s.identity == nil || input.Organization != "" || input.InvitationID != "" ||
		(r.URL.Path == "/api/status" && (input.Code != "" || input.ConfirmRecovery)) {
		s.reply(w, http.StatusConflict, "Prepare an identity first; saved invitation details cannot be overridden.")
		return
	}
	if time.Now().Before(s.next) {
		s.reply(w, http.StatusTooManyRequests, "Wait before contacting the enrollment service again.")
		return
	}
	claim := r.URL.Path == "/api/claim"
	if claim && (!s.checked || s.status != nil || (s.uncertain && (!s.recovery || !input.ConfirmRecovery)) || len(input.Code) != 26) {
		s.reply(w, http.StatusConflict, "Check status first, or verify the code. An uncertain claim must not be replayed.")
		return
	}
	metadata := s.identity.Metadata()
	ctx, cancel := context.WithTimeout(r.Context(), 32*time.Second)
	defer cancel()
	var status DeviceStatus
	var err error
	s.recovery = false
	if claim {
		s.uncertain = true
		status, err = s.client.Claim(ctx, metadata.Organization, metadata.InvitationID, input.Code)
	} else {
		status, err = s.client.Status(ctx, metadata.Organization, metadata.InvitationID)
	}
	s.next = time.Now().Add(10 * time.Second)
	if err != nil {
		var remote *RemoteError
		if errors.As(err, &remote) {
			if remote.RetryAfter > 10*time.Second {
				s.next = time.Now().Add(remote.RetryAfter)
			}
			// The device service intentionally conflates unavailable invitations
			// and unknown keys. A denial does not establish an unclaimed state.
			if !claim && remote.Status == http.StatusForbidden && s.status == nil {
				s.checked, s.uncertain, s.recovery = true, true, true
				s.reply(w, http.StatusForbidden, "Status denied; claim outcome remains unknown. Confirm with your administrator that this invitation is valid for this key before explicitly submitting the code. The saved key will not be replaced.")
				return
			}
		}
		s.uncertain = true
		s.reply(w, http.StatusBadGateway, "Enrollment response unavailable or invalid. Outcome is uncertain; check fresh status after the retry delay. Do not create a replacement identity.")
		return
	}
	s.checked, s.uncertain, s.recovery, s.status = true, false, false, &status
	message := "Enrollment state: " + status.State + "."
	if status.State == "approved" {
		message = "Administrator approved this request. Credentials are not issued and ZPR connectivity is not established."
	} else if status.State == "pending_approval" {
		message = "Waiting for administrator approval. Share both key fingerprints through your authenticated verification channel."
	} else {
		message += " Contact the administrator; no credentials were issued."
	}
	s.reply(w, http.StatusOK, message)
}

func (s *SetupServer) reply(w http.ResponseWriter, code int, message string) {
	view := setupView{Audience: s.config.Audience, Status: s.status, Uncertain: s.uncertain, Message: message}
	view.KeyProtection = "Software development key, not hardware-backed or encrypted at rest."
	if runtime.GOOS == "windows" {
		view.KeyProtection = "Software development key protected at rest by Windows user-bound DPAPI, not hardware-backed. Other Windows users require separate enrollment."
	}
	if s.config.KeyProtection == "macos-keychain" {
		view.KeyProtection = "Software development key stored in the current user's macOS Keychain, not Secure Enclave-backed or device attestation. Keychain access is required to resume."
	}
	if s.identity != nil {
		metadata := s.identity.Metadata()
		view.Metadata, view.Fingerprint = &metadata, s.client.Fingerprint()
		view.RuntimeFingerprint = s.client.runtimeFP
		view.CanClaim = s.checked && (!s.uncertain || s.recovery) && s.status == nil
		view.ConfirmRecovery = s.recovery
	}
	if time.Now().Before(s.next) {
		view.RetryAfter = int(math.Ceil(time.Until(s.next).Seconds()))
		w.Header().Set("Retry-After", strconv.Itoa(view.RetryAfter))
	}
	apiJSON(w, code, view)
}
