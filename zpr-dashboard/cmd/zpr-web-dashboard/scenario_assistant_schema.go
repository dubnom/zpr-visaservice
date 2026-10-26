//go:build !production

package main

import "sort"

func scenarioAssistantResponseSchema() map[string]any {
	object := func(properties map[string]any) map[string]any {
		required := make([]string, 0, len(properties))
		for name := range properties {
			required = append(required, name)
		}
		sort.Strings(required)
		return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	}
	text := map[string]any{"type": "string"}
	texts := map[string]any{"type": "array", "items": text}
	array := func(item map[string]any) map[string]any {
		return map[string]any{"type": "array", "items": item}
	}
	step := object(map[string]any{
		"id": text, "after": texts, "action": text, "machine": text, "user": text,
		"workloads": texts, "component": text, "target": text, "expected": text,
		"timeout_seconds": map[string]any{"type": "integer"}, "client_count": map[string]any{"type": "integer"},
	})
	identity := object(map[string]any{"name": text, "auth": text})
	component := object(map[string]any{
		"name": text, "kind": text, "node_substrate": text, "machine": text,
		"namespace": text, "address": text, "target": text, "agent": text,
		"gateway_upstream": text, "identities": array(identity),
	})
	topology := object(map[string]any{
		"nodes":      array(object(map[string]any{"id": text, "zpr_address": text, "substrate": text})),
		"links":      array(object(map[string]any{"id": text, "from": text, "to": text})),
		"components": array(component),
	})
	scenario := object(map[string]any{
		"id": text, "organization_id": text, "folder": text, "name": text,
		"parallel": map[string]any{"type": "boolean"}, "description": text,
		"topology": topology, "steps": array(step), "cleanup": array(step),
	})
	return object(map[string]any{
		"answer": text,
		"proposal": map[string]any{"anyOf": []any{
			map[string]any{"type": "null"},
			object(map[string]any{
				"scenario": scenario,
			}),
		}},
	})
}

func scenarioAssistantPatchResponseSchema() map[string]any {
	object := func(properties map[string]any) map[string]any {
		required := make([]string, 0, len(properties))
		for name := range properties {
			required = append(required, name)
		}
		sort.Strings(required)
		return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	}
	text := map[string]any{"type": "string"}
	patchStep := object(map[string]any{
		"action": text, "machine": text, "user": text, "workloads": map[string]any{"type": "array", "items": text},
		"component": text, "target": text, "expected": text,
		"timeout_seconds": map[string]any{"type": "integer"}, "client_count": map[string]any{"type": "integer"},
	})
	patch := object(map[string]any{
		"name": text, "description": text,
		"append_step":           patchStep,
		"concurrent_after_step": map[string]any{"type": "integer"},
	})
	return object(map[string]any{
		"answer":   text,
		"proposal": object(map[string]any{"scenario_patch": patch}),
	})
}
