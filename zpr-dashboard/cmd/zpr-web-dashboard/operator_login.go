package main

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"strings"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatorweb"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
	"neboagency.com/zpr-dashborad/internal/systemconfig"
)

type controlRoomSecurity struct {
	origin      string
	tls         *tls.Config
	allowedPeer net.IP
	auth        *operatorauth.Auth
	management  bool
}

type operatorWebConfig struct {
	application, certPath, keyPath, origin string
	configPath, secretPath, caPath         string
	trustedPeerIP                          string
	managementIP, organization             string
	budgetApprovalAccess                   *systemconfig.BudgetApprovalAccess
}

func readOperatorFile(path string, limit int64, private bool) ([]byte, error) {
	return servicehttp.ReadOperatorFile(path, limit, private)
}

func controlRoomWebConfig() operatorWebConfig {
	return operatorWebConfig{
		application: "Control Room", certPath: os.Getenv("ZPR_CONTROL_ROOM_CERT_FILE"),
		keyPath: os.Getenv("ZPR_CONTROL_ROOM_KEY_FILE"), origin: os.Getenv("ZPR_CONTROL_ROOM_ORIGIN"),
		configPath: os.Getenv("ZPR_OPERATOR_OIDC_CONFIG_FILE"), secretPath: os.Getenv("ZPR_OPERATOR_OIDC_SECRET_FILE"),
		caPath:               os.Getenv("ZPR_OPERATOR_OIDC_CA_FILE"),
		trustedPeerIP:        os.Getenv("ZPR_CONTROL_ROOM_OPERATOR_TRUSTED_PEER_IP"),
		managementIP:         os.Getenv("ZPR_CONTROL_ROOM_MANAGEMENT_IP"),
		organization:         os.Getenv("ZPR_CONTROL_ROOM_ORGANIZATION_ID"),
		budgetApprovalAccess: configuredBudgetApprovalAccess(),
	}
}

func configuredBudgetApprovalAccess() *systemconfig.BudgetApprovalAccess {
	serviceURL := strings.TrimSpace(os.Getenv("ZPR_CONTROL_SERVICE_URL"))
	signerConfig := strings.TrimSpace(os.Getenv("ZPR_OPERATOR_DELEGATION_SIGNER_FILE"))
	signerKey := strings.TrimSpace(os.Getenv("ZPR_OPERATOR_DELEGATION_KEY_FILE"))
	audience := strings.TrimSpace(os.Getenv("ZPR_OPERATOR_DELEGATION_AUDIENCE"))
	hasExplicitAudience := audience != ""
	if audience == "" {
		audience = serviceURL
	}
	if signerConfig == "" && signerKey == "" && !hasExplicitAudience {
		return nil
	}
	return &systemconfig.BudgetApprovalAccess{
		ControlServiceURL: serviceURL,
		SignerConfigFile:  signerConfig,
		SignerKeyFile:     signerKey,
		SignerAudience:    audience,
	}
}

func configuredControlRoomSecurity(ctx context.Context, listen string) (*controlRoomSecurity, error) {
	config := controlRoomWebConfig()
	return loadOperatorWebSecurity(ctx, listen, config, newOperatorAuthFactory(config.caPath))
}

func loadControlRoomSecurity(ctx context.Context, listen string, newAuth func(context.Context, operatorauth.Config, string) (*operatorauth.Auth, error)) (*controlRoomSecurity, error) {
	return loadOperatorWebSecurity(ctx, listen, controlRoomWebConfig(), newAuth)
}

func configuredSimulatorSecurity(ctx context.Context, listen string) (*controlRoomSecurity, error) {
	config := operatorWebConfig{
		application: "Simulator", certPath: os.Getenv("ZPR_SIMULATOR_OPERATOR_CERT_FILE"),
		keyPath: os.Getenv("ZPR_SIMULATOR_OPERATOR_KEY_FILE"), origin: os.Getenv("ZPR_SIMULATOR_OPERATOR_ORIGIN"),
		configPath:    os.Getenv("ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE"),
		secretPath:    os.Getenv("ZPR_SIMULATOR_OPERATOR_OIDC_SECRET_FILE"),
		caPath:        os.Getenv("ZPR_SIMULATOR_OPERATOR_OIDC_CA_FILE"),
		trustedPeerIP: os.Getenv("ZPR_SIMULATOR_OPERATOR_TRUSTED_PEER_IP"),
	}
	return loadOperatorWebSecurity(ctx, listen, config, newOperatorAuthFactory(config.caPath))
}

func newOperatorAuthFactory(caPath string) func(context.Context, operatorauth.Config, string) (*operatorauth.Auth, error) {
	return operatorweb.NewAuthFactory(caPath)
}

func loadOperatorWebSecurity(ctx context.Context, listen string, config operatorWebConfig,
	newAuth func(context.Context, operatorauth.Config, string) (*operatorauth.Auth, error),
) (*controlRoomSecurity, error) {
	security, err := operatorweb.Load(ctx, listen, operatorweb.Config{
		Application: config.application, CertPath: config.certPath, KeyPath: config.keyPath, Origin: config.origin,
		ConfigPath: config.configPath, SecretPath: config.secretPath, CAPath: config.caPath,
		TrustedPeerIP: config.trustedPeerIP, ManagementIP: config.managementIP, Organization: config.organization,
		BudgetApprovalAccess: config.budgetApprovalAccess,
	}, newAuth)
	if err != nil {
		return nil, err
	}
	return &controlRoomSecurity{
		origin: security.Origin, tls: security.TLS, allowedPeer: security.AllowedPeer,
		auth: security.Auth, management: security.Management,
	}, nil
}

func (s *controlRoomSecurity) boundary() *operatorweb.Security {
	return &operatorweb.Security{Origin: s.origin, TLS: s.tls, AllowedPeer: s.allowedPeer, Auth: s.auth, Management: s.management}
}

func (s *controlRoomSecurity) close() {
	s.boundary().Close()
}

func (s *controlRoomSecurity) protect(next http.Handler) http.Handler {
	return s.boundary().Protect(next, writePolicyError)
}

func (s *controlRoomSecurity) register(mux *http.ServeMux) {
	s.boundary().Register(mux, writeJSON, writePolicyError)
}
