//go:build production

package main

// Simulator assistant artifacts are not embedded in the production build.
const (
	directoryAssistantSkill    = ""
	scenarioAssistantSkill     = ""
	organizationAssistantSkill = ""
)
