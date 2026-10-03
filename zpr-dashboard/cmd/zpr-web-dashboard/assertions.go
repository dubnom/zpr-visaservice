package main

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/scanner"
	"time"
)

const maxAssertionSource = 64 << 10
const maxAssertionRules = 200

type assertionDirectory struct {
	People []string            `json:"people"`
	Groups map[string][]string `json:"groups"`
}

type assertionRule struct {
	Line     int      `json:"line"`
	Kind     string   `json:"kind"`
	Group    string   `json:"group,omitempty"`
	Groups   []string `json:"groups,omitempty"`
	Scope    string   `json:"scope,omitempty"`
	Operator string   `json:"operator,omitempty"`
	Limit    int      `json:"limit,omitempty"`
}

type assertionResult struct {
	Rule       assertionRule `json:"rule"`
	Status     string        `json:"status"`
	Checked    int           `json:"checked"`
	Violations int           `json:"violations"`
	Subjects   []string      `json:"subjects"`
	Message    string        `json:"message"`
}

type assertionRun struct {
	Revision   int               `json:"revision"`
	Draft      bool              `json:"draft"`
	StartedAt  time.Time         `json:"started_at"`
	FinishedAt time.Time         `json:"finished_at"`
	Status     string            `json:"status"`
	Error      string            `json:"error,omitempty"`
	Results    []assertionResult `json:"results"`
}

type assertionParser struct {
	scanner scanner.Scanner
	token   rune
	text    string
	err     error
}

func parseAssertions(source string) ([]assertionRule, error) {
	if len(source) > maxAssertionSource {
		return nil, errors.New("assertion source exceeds 64 KiB")
	}
	parser := &assertionParser{}
	parser.scanner.Init(strings.NewReader(source))
	parser.scanner.Mode = scanner.ScanIdents | scanner.ScanInts | scanner.ScanStrings | scanner.ScanComments | scanner.SkipComments
	parser.scanner.Error = func(input *scanner.Scanner, message string) {
		if parser.err == nil {
			parser.err = fmt.Errorf("line %d: %s", input.Position.Line, message)
		}
	}
	parser.next()
	rules := make([]assertionRule, 0)
	for parser.token != scanner.EOF && parser.err == nil {
		if len(rules) >= maxAssertionRules {
			return nil, errors.New("at most 200 assertions are allowed")
		}
		rule := assertionRule{Line: parser.scanner.Position.Line}
		switch parser.text {
		case "group":
			parser.next()
			rule.Kind, rule.Group = "group", parser.name()
			parser.expect("members")
			parser.cardinality(&rule)
		case "each":
			parser.next()
			parser.expect("group")
			parser.expect("members")
			rule.Kind = "each_group"
			parser.cardinality(&rule)
		case "people":
			parser.next()
			if parser.text == "in" {
				parser.next()
				rule.Scope = parser.name()
			}
			if parser.text != "exactly_one" && parser.text != "not_both" {
				parser.fail("expected exactly_one or not_both")
				break
			}
			rule.Kind = parser.text
			parser.next()
			rule.Groups = parser.groupList()
			if rule.Kind == "not_both" && len(rule.Groups) != 2 {
				parser.fail("not_both requires exactly two groups")
			}
		default:
			parser.fail("expected group, each group, or people")
		}
		parser.expect(";")
		rules = append(rules, rule)
	}
	return rules, parser.err
}

func (parser *assertionParser) next() {
	parser.token = parser.scanner.Scan()
	parser.text = parser.scanner.TokenText()
}

func (parser *assertionParser) fail(message string) {
	if parser.err == nil {
		parser.err = fmt.Errorf("line %d, column %d: %s", parser.scanner.Position.Line, parser.scanner.Position.Column, message)
	}
}

func (parser *assertionParser) expect(text string) {
	if parser.err != nil {
		return
	}
	if parser.text != text {
		parser.fail("expected " + text)
		return
	}
	parser.next()
}

func (parser *assertionParser) name() string {
	if parser.err != nil {
		return ""
	}
	if parser.token != scanner.String {
		parser.fail("expected a double-quoted group name")
		return ""
	}
	name, err := strconv.Unquote(parser.text)
	if err != nil || strings.TrimSpace(name) == "" || len(name) > 200 {
		parser.fail("group names must contain 1 to 200 bytes")
	}
	parser.next()
	return name
}

func (parser *assertionParser) cardinality(rule *assertionRule) {
	if parser.err != nil {
		return
	}
	operator := parser.text
	parser.next()
	if parser.text == "=" {
		operator += "="
		parser.next()
	}
	switch operator {
	case ">", ">=", "<", "<=", "==", "!=":
		rule.Operator = operator
	default:
		parser.fail("expected >, >=, <, <=, ==, or !=")
		return
	}
	limit, err := strconv.Atoi(parser.text)
	if parser.token != scanner.Int || err != nil || limit < 0 || limit > 1000000 {
		parser.fail("member count must be an integer between 0 and 1000000")
		return
	}
	rule.Limit = limit
	parser.next()
}

func (parser *assertionParser) groupList() []string {
	parser.expect("[")
	groups := make([]string, 0)
	seen := make(map[string]bool)
	for parser.err == nil {
		name := parser.name()
		if parser.err != nil {
			break
		}
		if seen[name] {
			parser.fail("group lists must not contain duplicates")
			break
		}
		seen[name] = true
		groups = append(groups, name)
		if len(groups) > 64 {
			parser.fail("group lists may contain at most 64 groups")
			break
		}
		if parser.text != "," {
			break
		}
		parser.next()
	}
	parser.expect("]")
	return groups
}

func evaluateAssertions(rules []assertionRule, directory assertionDirectory) []assertionResult {
	people := make(map[string]bool)
	for _, person := range directory.People {
		people[person] = true
	}
	groups := make(map[string]map[string]bool)
	for name, members := range directory.Groups {
		groups[name] = make(map[string]bool)
		for _, member := range members {
			groups[name][member] = true
		}
	}
	results := make([]assertionResult, 0, len(rules))
	for _, rule := range rules {
		result := assertionResult{Rule: rule, Status: "pass", Subjects: []string{}}
		required := append([]string(nil), rule.Groups...)
		if rule.Kind == "group" {
			required = append(required, rule.Group)
		}
		if rule.Scope != "" {
			required = append(required, rule.Scope)
		}
		for _, name := range required {
			if _, exists := groups[name]; !exists {
				result.Status, result.Message = "error", "Unknown group: "+name
				break
			}
		}
		if result.Status == "error" {
			results = append(results, result)
			continue
		}
		check := func(subject string, valid bool) {
			result.Checked++
			if !valid {
				result.Status = "fail"
				result.Violations++
				if len(result.Subjects) < 100 {
					result.Subjects = append(result.Subjects, subject)
				}
			}
		}
		if rule.Kind == "group" || rule.Kind == "each_group" {
			names := []string{rule.Group}
			if rule.Kind == "each_group" {
				names = make([]string, 0, len(groups))
				for name := range groups {
					names = append(names, name)
				}
				sort.Strings(names)
			}
			for _, name := range names {
				count := len(groups[name])
				valid := false
				switch rule.Operator {
				case ">":
					valid = count > rule.Limit
				case ">=":
					valid = count >= rule.Limit
				case "<":
					valid = count < rule.Limit
				case "<=":
					valid = count <= rule.Limit
				case "==":
					valid = count == rule.Limit
				case "!=":
					valid = count != rule.Limit
				}
				check(fmt.Sprintf("%s (%d members)", name, count), valid)
			}
		} else {
			selected := people
			if rule.Scope != "" {
				selected = groups[rule.Scope]
			}
			names := make([]string, 0, len(selected))
			for name := range selected {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, person := range names {
				count := 0
				for _, name := range rule.Groups {
					if groups[name][person] {
						count++
					}
				}
				check(person, rule.Kind == "exactly_one" && count == 1 || rule.Kind == "not_both" && count < 2)
			}
		}
		if result.Checked == 0 {
			result.Status, result.Message = "error", "No subjects found in the selected scope"
		} else if result.Message == "" {
			result.Message = fmt.Sprintf("%d checked; %d violations", result.Checked, result.Violations)
		}
		results = append(results, result)
	}
	return results
}
