package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/budgetauthority"
)

var budgetResolverEnvironment = []string{
	budgetauthority.ResolverURLEnv,
	budgetauthority.ResolverClientCertFileEnv,
	budgetauthority.ResolverClientKeyFileEnv,
	budgetauthority.ResolverCAFileEnv,
	budgetauthority.ResolverTLSServerNameEnv,
}

func clearBudgetResolverEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range budgetResolverEnvironment {
		value, wasSet := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if wasSet {
				if err := os.Setenv(name, value); err != nil {
					t.Errorf("restore environment variable %s: %v", name, err)
				}
			} else {
				if err := os.Unsetenv(name); err != nil {
					t.Errorf("clear environment variable %s: %v", name, err)
				}
			}
		})
	}
}

func writeBudgetResolverCredentials(t *testing.T) (string, string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), DNSNames: []string{"localhost"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage:    x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	directory := t.TempDir()
	certPath := filepath.Join(directory, "client.pem")
	keyPath := filepath.Join(directory, "client-key.pem")
	caPath := filepath.Join(directory, "ca.pem")
	for path, content := range map[string][]byte{
		certPath: certPEM,
		keyPath:  keyPEM,
		caPath:   certPEM,
	} {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return certPath, keyPath, caPath
}

func TestSimulatorScenarioBudgetResolverConfiguration(t *testing.T) {
	t.Run("all settings absent defaults to unavailable", func(t *testing.T) {
		clearBudgetResolverEnvironment(t)
		manager := newSimulatorScenarioManager()
		_, err := manager.budgetEvidenceResolver.ResolveCurrent(context.Background(), budgetauthority.Scope{})
		if err != budgetauthority.ErrUnavailable {
			t.Fatalf("ResolveCurrent() error = %v, want %v", err, budgetauthority.ErrUnavailable)
		}
	})

	t.Run("partial settings fail explicitly", func(t *testing.T) {
		clearBudgetResolverEnvironment(t)
		if err := os.Setenv(budgetauthority.ResolverURLEnv, "https://control-service.example"); err != nil {
			t.Fatal(err)
		}
		manager := newSimulatorScenarioManager()
		_, err := manager.budgetEvidenceResolver.ResolveCurrent(context.Background(), budgetauthority.Scope{})
		if err == nil || err == budgetauthority.ErrUnavailable {
			t.Fatalf("ResolveCurrent() error = %v, want explicit configuration error", err)
		}
	})

	t.Run("fully configured settings select HTTP resolver", func(t *testing.T) {
		clearBudgetResolverEnvironment(t)
		certPath, keyPath, caPath := writeBudgetResolverCredentials(t)
		values := map[string]string{
			budgetauthority.ResolverURLEnv:            "https://control-service.example",
			budgetauthority.ResolverClientCertFileEnv: certPath,
			budgetauthority.ResolverClientKeyFileEnv:  keyPath,
			budgetauthority.ResolverCAFileEnv:         caPath,
			budgetauthority.ResolverTLSServerNameEnv:  "control-service.example",
		}
		for name, value := range values {
			if err := os.Setenv(name, value); err != nil {
				t.Fatal(err)
			}
		}
		manager := newSimulatorScenarioManager()
		if _, ok := manager.budgetEvidenceResolver.(*budgetauthority.HTTPResolver); !ok {
			t.Fatalf("default resolver type = %T, want *budgetauthority.HTTPResolver", manager.budgetEvidenceResolver)
		}
	})

	t.Run("invalid settings do not fall back to unavailable", func(t *testing.T) {
		clearBudgetResolverEnvironment(t)
		for name, value := range map[string]string{
			budgetauthority.ResolverURLEnv:            "http://control-service.example",
			budgetauthority.ResolverClientCertFileEnv: "client.pem",
			budgetauthority.ResolverClientKeyFileEnv:  "client-key.pem",
			budgetauthority.ResolverCAFileEnv:         "ca.pem",
		} {
			if err := os.Setenv(name, value); err != nil {
				t.Fatal(err)
			}
		}
		manager := newSimulatorScenarioManager()
		_, err := manager.budgetEvidenceResolver.ResolveCurrent(context.Background(), budgetauthority.Scope{})
		if err == nil || err == budgetauthority.ErrUnavailable {
			t.Fatalf("ResolveCurrent() error = %v, want explicit configuration error", err)
		}
	})
}

func TestSimulatorScenarioRunContextCancellationPreservesApproval(t *testing.T) {
	manager := newSimulatorScenarioManager()
	approvalStore := newSimulatorScenarioPlanApprovalStore()
	previousApprovalStore := simulatorScenarioPlanApprovals
	simulatorScenarioPlanApprovals = approvalStore
	t.Cleanup(func() { simulatorScenarioPlanApprovals = previousApprovalStore })

	scenario, manifest, organization := simulatorScenarioPlanRunContractInputs()
	approvalID, planDigest := issueApprovedSimulatorScenarioPlan(t, approvalStore, scenario, manifest, organization)
	resolverStarted := make(chan struct{})
	setSimulatorBudgetEvidenceResolver(manager, simulatorBudgetEvidenceResolverFunc(
		func(ctx context.Context, _ budgetauthority.Scope) (budgetauthority.Evidence, error) {
			close(resolverStarted)
			<-ctx.Done()
			return budgetauthority.Evidence{}, ctx.Err()
		},
	))

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- manager.startApprovedVersionedContext(
			ctx, scenario, manifest, organization, nil,
			organization.ID, 1, approvalID, planDigest,
		)
	}()
	select {
	case <-resolverStarted:
	case <-time.After(time.Second):
		t.Fatal("budget resolver was not called")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("startApprovedVersionedContext() error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("scenario start did not return after context cancellation")
	}

	if run := manager.snapshot(); run.State != "idle" || run.RunID != "" || run.RunNamespace != "" {
		t.Fatalf("cancelled start allocated run state: %+v", run)
	}
	approvalStore.mu.Lock()
	_, approvalAvailable := approvalStore.byID[approvalID]
	approvalStore.mu.Unlock()
	if !approvalAvailable {
		t.Fatal("cancelled start consumed the local approval")
	}
}
