package main

import (
	"strings"
	"testing"
)

func TestNewSimulatorRunNamespaceCanonicalTuple(t *testing.T) {
	namespace, err := NewSimulatorRunNamespace("org-one", "scenario-one", "run-one")
	if err != nil {
		t.Fatalf("NewSimulatorRunNamespace returned an error: %v", err)
	}
	if got, want := namespace.Canonical(), "org-one/scenario-one/run-one"; got != want {
		t.Fatalf("Canonical() = %q, want %q", got, want)
	}
}

func TestSimulatorRunNamespaceDifferentComponentsDoNotCollide(t *testing.T) {
	inputs := [][3]string{
		{"org-one", "scenario-one", "run-one"},
		{"org-two", "scenario-one", "run-one"},
		{"org-one", "scenario-two", "run-one"},
		{"org-one", "scenario-one", "run-two"},
	}
	seen := make(map[string][3]string, len(inputs))
	for _, input := range inputs {
		namespace, err := NewSimulatorRunNamespace(input[0], input[1], input[2])
		if err != nil {
			t.Fatalf("NewSimulatorRunNamespace(%q, %q, %q) returned an error: %v",
				input[0], input[1], input[2], err)
		}
		canonical := namespace.Canonical()
		if previous, exists := seen[canonical]; exists {
			t.Fatalf("inputs %q and %q collide as %q", previous, input, canonical)
		}
		seen[canonical] = input
	}
}

func TestNewSimulatorRunNamespaceRejectsInvalidIDs(t *testing.T) {
	invalidIDs := []string{
		"",
		"Uppercase",
		"1-leading-digit",
		"contains/slash",
		"contains_underscore",
		"contains space",
		strings.Repeat("a", maxSimulatorRunNamespaceIDLength+1),
	}
	for _, invalidID := range invalidIDs {
		t.Run(invalidID, func(t *testing.T) {
			if _, err := NewSimulatorRunNamespace(invalidID, "scenario-one", "run-one"); err == nil {
				t.Fatal("NewSimulatorRunNamespace accepted an invalid organization ID")
			}
			if _, err := NewSimulatorRunNamespace("org-one", invalidID, "run-one"); err == nil {
				t.Fatal("NewSimulatorRunNamespace accepted an invalid scenario ID")
			}
			if _, err := NewSimulatorRunNamespace("org-one", "scenario-one", invalidID); err == nil {
				t.Fatal("NewSimulatorRunNamespace accepted an invalid run ID")
			}
		})
	}
}

func TestSimulatorRunNamespaceCanonicalLengthIsBounded(t *testing.T) {
	component := strings.Repeat("a", maxSimulatorRunNamespaceIDLength)
	namespace, err := NewSimulatorRunNamespace(component, component, component)
	if err != nil {
		t.Fatalf("maximum-length IDs were rejected: %v", err)
	}
	if got := len(namespace.Canonical()); got != maxSimulatorRunNamespaceCanonicalLength {
		t.Fatalf("canonical length = %d, want %d", got, maxSimulatorRunNamespaceCanonicalLength)
	}
}
