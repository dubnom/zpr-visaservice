//go:build !production

package main

import (
	"context"
	"errors"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

type simulatorAuthorizationMode uint8

const (
	simulatorAuthorizationLocal simulatorAuthorizationMode = iota + 1
	simulatorAuthorizationNamed
)

type simulatorAuthorizationModeContextKey struct{}
type simulatorScenarioRevalidatorContextKey struct{}
type simulatorScenarioRevalidator func(context.Context) (operatorauth.Identity, error)

type simulatorScenarioPrincipal struct {
	mode    simulatorAuthorizationMode
	issuer  string
	subject string
}

const simulatorScenarioOperatorDeniedMessage = "Scenario authorization is unavailable or denied; check the current operator session, scenario.run and approval.self for this organization."

var errSimulatorScenarioOperatorDenied = errors.New("current scenario operator authorization is unavailable or denied")

func withSimulatorAuthorizationMode(ctx context.Context, mode simulatorAuthorizationMode) context.Context {
	return context.WithValue(ctx, simulatorAuthorizationModeContextKey{}, mode)
}

func (principal simulatorScenarioPrincipal) valid() bool {
	switch principal.mode {
	case simulatorAuthorizationLocal:
		return principal.issuer == "" && principal.subject == ""
	case simulatorAuthorizationNamed:
		return operatorauth.ValidPrincipal(operatorauth.Identity{
			Issuer: principal.issuer, Subject: principal.subject,
		})
	default:
		return false
	}
}

func simulatorScenarioPrincipalFromContext(ctx context.Context) (simulatorScenarioPrincipal, error) {
	if ctx == nil {
		return simulatorScenarioPrincipal{}, errSimulatorScenarioOperatorDenied
	}
	mode, ok := ctx.Value(simulatorAuthorizationModeContextKey{}).(simulatorAuthorizationMode)
	if !ok {
		return simulatorScenarioPrincipal{}, errSimulatorScenarioOperatorDenied
	}
	identity, present := simulatorOperatorIdentity(ctx)
	if mode == simulatorAuthorizationLocal && !present {
		return simulatorScenarioPrincipal{mode: mode}, nil
	}
	if mode != simulatorAuthorizationNamed || !present || !operatorauth.ValidPrincipal(identity) {
		return simulatorScenarioPrincipal{}, errSimulatorScenarioOperatorDenied
	}
	return simulatorScenarioPrincipal{
		mode: mode, issuer: identity.Issuer, subject: identity.Subject,
	}, nil
}

func revalidateSimulatorScenarioPrincipal(ctx context.Context, principal simulatorScenarioPrincipal) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := simulatorScenarioPrincipalFromContext(ctx)
	if err != nil || current != principal {
		return errSimulatorScenarioOperatorDenied
	}
	if principal.mode == simulatorAuthorizationLocal {
		return nil
	}
	revalidate, ok := ctx.Value(simulatorScenarioRevalidatorContextKey{}).(simulatorScenarioRevalidator)
	if !ok || revalidate == nil {
		return errSimulatorScenarioOperatorDenied
	}
	identity, err := revalidate(ctx)
	if err != nil || !operatorauth.ValidPrincipal(identity) ||
		identity.Issuer != principal.issuer || identity.Subject != principal.subject {
		return errSimulatorScenarioOperatorDenied
	}
	return ctx.Err()
}
