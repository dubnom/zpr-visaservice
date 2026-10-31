package setupmode

import "errors"

const Environment = "SIMULATION_SETUP_MODE"

type Mode uint8

const (
	Invalid Mode = iota
	LegacyLauncher
	OrdinaryClient
)

func Parse(value string) (Mode, error) {
	switch value {
	case "", "legacy-launcher":
		return LegacyLauncher, nil
	case "ordinary-client":
		return OrdinaryClient, nil
	default:
		return Invalid, errors.New("invalid SIMULATION_SETUP_MODE; expected legacy-launcher or ordinary-client")
	}
}
