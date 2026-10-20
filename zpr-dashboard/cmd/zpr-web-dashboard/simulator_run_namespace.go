package main

import "fmt"

const (
	maxSimulatorRunNamespaceIDLength        = 63
	maxSimulatorRunNamespaceCanonicalLength = 3*maxSimulatorRunNamespaceIDLength + 2
)

// SimulatorRunNamespace identifies one simulator run within an organization
// and scenario. It does not create resources or enforce run concurrency.
type SimulatorRunNamespace struct {
	organizationID string
	scenarioID     string
	runID          string
}

// NewSimulatorRunNamespace validates the IDs used to identify one simulator run.
func NewSimulatorRunNamespace(orgID, scenarioID, runID string) (SimulatorRunNamespace, error) {
	for _, component := range []struct {
		name string
		id   string
	}{
		{name: "organization ID", id: orgID},
		{name: "scenario ID", id: scenarioID},
		{name: "run ID", id: runID},
	} {
		if !validScenarioID(component.id) || len(component.id) > maxSimulatorRunNamespaceIDLength {
			return SimulatorRunNamespace{}, fmt.Errorf(
				"%s must be a valid lowercase ID of at most %d characters",
				component.name,
				maxSimulatorRunNamespaceIDLength,
			)
		}
	}

	namespace := SimulatorRunNamespace{
		organizationID: orgID,
		scenarioID:     scenarioID,
		runID:          runID,
	}
	if len(namespace.Canonical()) > maxSimulatorRunNamespaceCanonicalLength {
		return SimulatorRunNamespace{}, fmt.Errorf(
			"canonical simulator run namespace exceeds %d characters",
			maxSimulatorRunNamespaceCanonicalLength,
		)
	}
	return namespace, nil
}

// Canonical returns the injective organization/scenario/run tuple.
func (namespace SimulatorRunNamespace) Canonical() string {
	return namespace.organizationID + "/" + namespace.scenarioID + "/" + namespace.runID
}
