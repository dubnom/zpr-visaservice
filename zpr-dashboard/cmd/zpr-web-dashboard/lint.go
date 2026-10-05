package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type lintDiagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Line     int    `json:"line"`
	Message  string `json:"message"`
}

func assertionExpressionComplexity(expression *assertionExpression) (int, int) {
	if expression == nil {
		return 0, 0
	}
	leftNodes, leftDepth := assertionExpressionComplexity(expression.left)
	rightNodes, rightDepth := assertionExpressionComplexity(expression.right)
	return leftNodes + rightNodes + 1, max(leftDepth, rightDepth) + 1
}

func assertionExpressionSignature(expression *assertionExpression) string {
	if expression == nil {
		return ""
	}
	return fmt.Sprintf("%q:%q:%q:%q:%q:(%s):(%s)", expression.kind, expression.text, expression.source, expression.subject, expression.attribute,
		assertionExpressionSignature(expression.left), assertionExpressionSignature(expression.right))
}

func lintAssertions(rules []assertionRule) []lintDiagnostic {
	warnings := []lintDiagnostic{}
	seen := make(map[string]int)
	equalities := make(map[string]assertionRule)
	warn := func(rule assertionRule, code, message string) {
		warnings = append(warnings, lintDiagnostic{Code: code, Severity: "warning", Line: rule.Line, Message: message})
	}
	for _, rule := range rules {
		canonical := rule
		canonical.Line = 0
		canonical.ExpressionText = ""
		encoded, _ := json.Marshal(canonical)
		signature := string(encoded) + assertionExpressionSignature(rule.Expression)
		if prior, duplicate := seen[signature]; duplicate {
			warn(rule, "ASSERT_DUPLICATE", fmt.Sprintf("Duplicates the check on line %d; remove the repeated assertion.", prior))
		} else {
			seen[signature] = rule.Line
		}
		nodes, depth := assertionExpressionComplexity(rule.Expression)
		if nodes > 24 || depth > 6 || len(rule.Groups) > 12 {
			warn(rule, "ASSERT_COMPLEXITY", fmt.Sprintf("Complex assertion (%d expression nodes, depth %d, %d groups); split it into smaller independently reviewable checks.", nodes, depth, len(rule.Groups)))
		}
		if (rule.Kind == "group" || rule.Kind == "each_group") && (rule.Operator == ">=" && rule.Limit == 0 || rule.Operator == "<=" && rule.Limit == 1000000) {
			warn(rule, "ASSERT_TRIVIAL", "This cardinality check provides little assurance; require a meaningful minimum or maximum.")
		}
		if rule.Kind == "people_attribute" && rule.Scope == "" {
			switch strings.ToLower(rule.Attribute) {
			case "mail", "title", "ou", "givenname", "sn", "employeetype", "employeenumber":
				warn(rule, "ASSERT_HUMAN_SCOPE", "This human-profile check applies to all returned identities, including machine/service accounts; scope it to an appropriate people group.")
			}
		}
		if rule.Expression == nil && rule.Operator == "==" {
			key := fmt.Sprintf("%q/%q/%q/%q/%q", rule.Kind, rule.Source, rule.Group, rule.Scope, strings.ToLower(rule.Attribute))
			prior := equalities[key]
			priorNumber, _ := json.Marshal(prior.Number)
			number, _ := json.Marshal(rule.Number)
			if prior.Line != 0 && (prior.Value != rule.Value || prior.Limit != rule.Limit || string(priorNumber) != string(number)) {
				warn(rule, "ASSERT_CONTRADICTION", fmt.Sprintf("Conflicts with an equality check on line %d for the same scope; review the intended invariant.", prior.Line))
			} else {
				equalities[key] = rule
			}
		}
	}
	return warnings
}

func splitCompilerLint(output string) (string, []lintDiagnostic) {
	lines := []string{}
	warnings := []lintDiagnostic{}
	for _, line := range strings.Split(output, "\n") {
		if encoded, found := strings.CutPrefix(line, "ZPR_LINT "); found {
			var warning lintDiagnostic
			if json.Unmarshal([]byte(encoded), &warning) == nil && warning.Severity == "warning" && warning.Line > 0 {
				warnings = append(warnings, warning)
				continue
			}
		}
		if strings.HasPrefix(strings.TrimSpace(line), "warning:") {
			warnings = append(warnings, lintDiagnostic{Code: "COMPILER_WARNING", Severity: "warning", Line: 1, Message: strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "warning:"))})
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), warnings
}
