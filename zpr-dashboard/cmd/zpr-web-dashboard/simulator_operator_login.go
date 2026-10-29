//go:build !production

package main

import (
	"context"
	"os"
)

func configuredSimulatorSecurity(ctx context.Context, listen string) (*controlRoomSecurity, error) {
	config := operatorWebConfig{
		application: "Simulator", certPath: os.Getenv("ZPR_SIMULATOR_OPERATOR_CERT_FILE"),
		keyPath: os.Getenv("ZPR_SIMULATOR_OPERATOR_KEY_FILE"), origin: os.Getenv("ZPR_SIMULATOR_OPERATOR_ORIGIN"),
		configPath:    os.Getenv("ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE"),
		secretPath:    os.Getenv("ZPR_SIMULATOR_OPERATOR_OIDC_SECRET_FILE"),
		caPath:        os.Getenv("ZPR_SIMULATOR_OPERATOR_OIDC_CA_FILE"),
		trustedPeerIP: os.Getenv("ZPR_SIMULATOR_OPERATOR_TRUSTED_PEER_IP"),
		directIP:      os.Getenv("ZPR_SIMULATOR_OPERATOR_DIRECT_IP"),
	}
	return loadOperatorWebSecurity(ctx, listen, config, newOperatorAuthFactory(config.caPath))
}
