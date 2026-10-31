//go:build !production

package main

import (
	"errors"
	"fmt"
	"os"

	"neboagency.com/zpr-dashborad/internal/simulator/setupmode"
)

var errSimulatorUnboundLegacyMachineRuntime = errors.New(
	"unbound legacy Simulator machine runtime is blocked in ordinary-client mode (U01-MACHINE-RUNTIME)",
)

func simulatorSetupMode() (setupmode.Mode, error) {
	return setupmode.Parse(os.Getenv(setupmode.Environment))
}

func requireLegacySimulatorMode(message string) error {
	mode, err := simulatorSetupMode()
	if err != nil {
		return err
	}
	if mode != setupmode.LegacyLauncher {
		return errors.New(message)
	}
	return nil
}

func requireLegacySimulatorSetup() error {
	return requireLegacySimulatorMode("legacy Simulator setup is blocked in ordinary-client mode until independent setup contracts are available (U01-ACTIVATION)")
}

func requireLegacySimulatorMachineRuntime() error {
	mode, err := simulatorSetupMode()
	if err != nil {
		return err
	}
	if mode != setupmode.LegacyLauncher {
		return errSimulatorUnboundLegacyMachineRuntime
	}
	return nil
}

func validateSimulatorScenarioMachineRuntime(
	organization simulatorOrganization,
	manifest simulatorManifest,
	machineIDs []string,
) error {
	mode, err := simulatorSetupMode()
	if err != nil || mode == setupmode.LegacyLauncher {
		return err
	}
	if len(machineIDs) == 0 {
		return nil
	}
	if organization.RetainedTopologyBinding != nil {
		if organization.RetainedMachineRuntime == nil {
			return errors.New("retained machine lifecycle consumer is not installed")
		}
		if err := validateSimulatorRetainedMachineRuntime(organization); err != nil {
			return err
		}
		for _, machineID := range machineIDs {
			if !isSimulatorRetainedMachine(machineID) {
				return fmt.Errorf("retained machine lifecycle does not support %q", machineID)
			}
		}
		return nil
	}
	if !organization.SyntheticLabOnly {
		return errSimulatorUnboundLegacyMachineRuntime
	}
	for _, machineID := range machineIDs {
		_, packaged, err := simulatorScenarioMachineContainerName(manifest, machineID)
		if err != nil {
			return fmt.Errorf("packaged scenario machine %q is unavailable: %w", machineID, err)
		}
		if !packaged {
			return errSimulatorUnboundLegacyMachineRuntime
		}
	}
	return nil
}
