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
	peopleLookup     map[string]bool
	memberCounts     map[string]int
	Sources          map[string]assertionDirectory  `json:"-"`
	People           []string                       `json:"people"`
	Groups           map[string][]string            `json:"groups"`
	Attributes       []string                       `json:"attributes,omitempty"`
	PersonAttributes map[string]map[string][]string `json:"person_attributes,omitempty"`
	GroupAttributes  map[string]map[string][]string `json:"group_attributes,omitempty"`
}

type assertionRule struct {
	Source         string               `json:"source,omitempty"`
	ExpressionText string               `json:"expression,omitempty"`
	Expression     *assertionExpression `json:"-"`
	Line           int                  `json:"line"`
	Kind           string               `json:"kind"`
	Group          string               `json:"group,omitempty"`
	Groups         []string             `json:"groups,omitempty"`
	Scope          string               `json:"scope,omitempty"`
	Operator       string               `json:"operator,omitempty"`
	Limit          int                  `json:"limit,omitempty"`
	Attribute      string               `json:"attribute,omitempty"`
	Value          string               `json:"value,omitempty"`
	Values         []string             `json:"values,omitempty"`
	Number         *int64               `json:"number,omitempty"`
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
	OrganizationID string            `json:"organization_id,omitempty"`
	Revision       int               `json:"revision"`
	Draft          bool              `json:"draft"`
	StartedAt      time.Time         `json:"started_at"`
	FinishedAt     time.Time         `json:"finished_at"`
	Status         string            `json:"status"`
	Error          string            `json:"error,omitempty"`
	Results        []assertionResult `json:"results"`
	Warnings       []lintDiagnostic  `json:"warnings,omitempty"`
}

type assertionParser struct {
	expressionNodes int
	expressionDepth int
	scanner         scanner.Scanner
	token           rune
	text            string
	err             error
}

func formatAssertionSource(source string) (string, error) {
	if _, err := parseAssertions(source); err != nil {
		return "", err
	}
	var input scanner.Scanner
	input.Init(strings.NewReader(source))
	input.Mode = scanner.ScanIdents | scanner.ScanInts | scanner.ScanFloats | scanner.ScanStrings | scanner.ScanComments
	var output strings.Builder
	previous := ""
	for token := input.Scan(); token != scanner.EOF; token = input.Scan() {
		text := input.TokenText()
		if token == scanner.Comment {
			if previous != "" {
				output.WriteByte('\n')
			}
			output.WriteString(text)
			output.WriteByte('\n')
			previous = ""
			continue
		}
		space := previous != "" && text != ";" && text != "," && text != ")" && text != "]" && text != "." && previous != "(" && previous != "[" && previous != "."
		if text == "(" && (previous == "source" || previous == "group" || previous == "attribute") {
			space = false
		}
		if text == "=" && strings.Contains("<>=!", previous) && len(previous) == 1 {
			space = false
		}
		if space {
			output.WriteByte(' ')
		}
		output.WriteString(text)
		if text == ";" {
			output.WriteByte('\n')
			previous = ""
		} else {
			previous = text
		}
	}
	formatted := strings.TrimSpace(output.String())
	if formatted != "" {
		formatted += "\n"
	}
	if _, err := parseAssertions(formatted); err != nil {
		return "", err
	}
	return formatted, nil
}

func parseAssertions(source string) ([]assertionRule, error) {
	if len(source) > maxAssertionSource {
		return nil, errors.New("assertion source exceeds 64 KiB")
	}
	parser := &assertionParser{}
	parser.scanner.Init(strings.NewReader(source))
	parser.scanner.Mode = scanner.ScanIdents | scanner.ScanInts | scanner.ScanFloats | scanner.ScanStrings | scanner.ScanComments | scanner.SkipComments
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
		case "assert":
			parser.next()
			start := parser.scanner.Position.Offset
			rule.Kind = "expression"
			rule.Expression = parser.expression(1)
			if parser.err == nil {
				rule.ExpressionText = strings.TrimSpace(source[start:parser.scanner.Position.Offset])
			}
		case "group":
			parser.next()
			rule.Kind, rule.Group = "group", parser.name()
			if parser.text == "from" {
				parser.next()
				rule.Source = parser.name()
			}
			if parser.text == "attribute" {
				rule.Kind = "group_attribute"
				parser.attribute(&rule)
			} else {
				parser.expect("members")
				parser.cardinality(&rule)
			}
		case "each":
			parser.next()
			parser.expect("group")
			rule.Kind = "each_group"
			if parser.text == "from" {
				parser.next()
				rule.Source = parser.name()
			}
			if parser.text == "attribute" {
				rule.Kind = "each_group_attribute"
				parser.attribute(&rule)
			} else {
				parser.expect("members")
				parser.cardinality(&rule)
			}
		case "people":
			parser.next()
			if parser.text == "from" {
				parser.next()
				rule.Source = parser.name()
			}
			if parser.text == "in" {
				parser.next()
				rule.Scope = parser.name()
			}
			if parser.text == "where" {
				parser.next()
				start := parser.scanner.Position.Offset
				rule.Kind = "people_expression"
				rule.Expression = parser.expression(1)
				if parser.err == nil {
					rule.ExpressionText = strings.TrimSpace(source[start:parser.scanner.Position.Offset])
				}
				break
			}
			if parser.text == "attribute" {
				rule.Kind = "people_attribute"
				parser.attribute(&rule)
				break
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
			parser.fail("expected assert, group, each group, or people")
		}
		if rule.Expression != nil && parser.err == nil {
			kind, err := validateAssertionExpression(rule.Expression, rule.Kind == "people_expression")
			if err != nil {
				parser.fail(err.Error())
			} else if kind != "boolean" {
				parser.fail("assertion expression must return a boolean comparison")
			}
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
		parser.fail("expected a double-quoted name")
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

func (parser *assertionParser) attribute(rule *assertionRule) {
	parser.expect("attribute")
	rule.Attribute = strings.ToLower(parser.name())
	if !validLDAPAttributeName(rule.Attribute) || assertionSensitiveAttribute(rule.Attribute) {
		parser.fail("attribute name is invalid or excluded for security")
		return
	}
	operator := parser.text
	parser.next()
	if operator == "present" || operator == "absent" {
		rule.Operator = operator
		return
	}
	if operator == "in" {
		rule.Operator = operator
		rule.Values = parser.groupList()
		return
	}
	if parser.text == "=" {
		operator += "="
		parser.next()
	}
	switch operator {
	case "==", "!=", ">", ">=", "<", "<=", "contains":
		rule.Operator = operator
	default:
		parser.fail("expected present, absent, ==, !=, >, >=, <, <=, in, or contains")
		return
	}
	if parser.token == scanner.String {
		value, err := strconv.Unquote(parser.text)
		if err != nil || len(value) > 4096 {
			parser.fail("attribute value exceeds limits")
			return
		}
		if operator != "==" && operator != "!=" && operator != "contains" {
			parser.fail("ordered attribute comparisons require an integer")
			return
		}
		rule.Value = value
		parser.next()
		return
	}
	if operator == "contains" {
		parser.fail("contains requires a double-quoted value")
		return
	}
	sign := ""
	if parser.text == "-" {
		sign = "-"
		parser.next()
	}
	number, err := strconv.ParseInt(sign+parser.text, 10, 64)
	if parser.token != scanner.Int || err != nil || number < -9007199254740991 || number > 9007199254740991 {
		parser.fail("attribute comparison requires a quoted value or safe integer")
		return
	}
	rule.Number = &number
	parser.next()
}

func assertionSensitiveAttribute(name string) bool {
	name = strings.ToLower(name)
	for _, excluded := range []string{"password", "passwd", "pwd", "token", "secret", "credential", "private", "key", "auth", "certificate"} {
		if strings.Contains(name, excluded) {
			return true
		}
	}
	return false
}

func compareAssertionAttribute(values []string, rule assertionRule) (bool, error) {
	if rule.Number != nil {
		for _, value := range values {
			if _, err := strconv.ParseInt(value, 10, 64); err != nil {
				return false, errors.New("Attribute has a non-integer value")
			}
		}
	}
	if rule.Operator == "present" {
		return len(values) > 0, nil
	}
	if rule.Operator == "absent" {
		return len(values) == 0, nil
	}
	if len(values) == 0 {
		return false, nil
	}
	if rule.Operator == "contains" {
		for _, value := range values {
			if value == rule.Value {
				return true, nil
			}
		}
		return false, nil
	}
	for _, value := range values {
		valid := false
		if rule.Number != nil {
			number, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return false, errors.New("Attribute has a non-integer value")
			}
			switch rule.Operator {
			case "==":
				valid = number == *rule.Number
			case "!=":
				valid = number != *rule.Number
			case ">":
				valid = number > *rule.Number
			case ">=":
				valid = number >= *rule.Number
			case "<":
				valid = number < *rule.Number
			case "<=":
				valid = number <= *rule.Number
			}
		} else {
			switch rule.Operator {
			case "==":
				valid = value == rule.Value
			case "!=":
				valid = value != rule.Value
			case "in":
				for _, allowed := range rule.Values {
					if value == allowed {
						valid = true
						break
					}
				}
			}
		}
		if !valid {
			return false, nil
		}
	}
	return true, nil
}

func evaluateAssertions(rules []assertionRule, directory assertionDirectory) []assertionResult {
	directory = indexAssertionDirectory(directory)
	if directory.Sources != nil {
		sources := make(map[string]assertionDirectory, len(directory.Sources))
		for name, selected := range directory.Sources {
			sources[name] = indexAssertionDirectory(selected)
		}
		directory.Sources = sources
	}
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
		if rule.Expression != nil {
			results = append(results, evaluateAssertionExpression(rule, directory))
			continue
		}
		if rule.Source != "" {
			selected, err := assertionExpressionDirectory(directory, rule.Source)
			if err != nil {
				results = append(results, assertionResult{Rule: rule, Status: "error", Message: err.Error(), Subjects: []string{}})
			} else {
				original := rule
				rule.Source = ""
				result := evaluateAssertions([]assertionRule{rule}, selected)[0]
				result.Rule = original
				results = append(results, result)
			}
			continue
		}
		result := assertionResult{Rule: rule, Status: "pass", Subjects: []string{}}
		required := append([]string(nil), rule.Groups...)
		if rule.Kind == "group" || rule.Kind == "group_attribute" {
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
		if rule.Attribute != "" {
			known := false
			for _, name := range directory.Attributes {
				if strings.EqualFold(name, rule.Attribute) {
					known = true
					break
				}
			}
			if !known {
				result.Status, result.Message = "error", "Attribute is not available from the approved source: "+rule.Attribute
				results = append(results, result)
				continue
			}
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
		if rule.Attribute != "" {
			selected := people
			attributes := directory.PersonAttributes
			if rule.Kind == "group_attribute" {
				selected = map[string]bool{rule.Group: true}
				attributes = directory.GroupAttributes
			}
			if rule.Kind == "each_group_attribute" {
				selected = make(map[string]bool)
				for name := range groups {
					selected[name] = true
				}
				attributes = directory.GroupAttributes
			}
			if rule.Scope != "" {
				selected = groups[rule.Scope]
			}
			names := make([]string, 0, len(selected))
			for name := range selected {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				valid, err := compareAssertionAttribute(attributes[name][rule.Attribute], rule)
				if err != nil {
					result.Status, result.Message = "error", err.Error()+" for "+name
					break
				}
				check(name, valid)
			}
		} else if rule.Kind == "group" || rule.Kind == "each_group" {
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
		if result.Checked == 0 && result.Status != "error" {
			result.Status, result.Message = "error", "No subjects found in the selected scope"
		} else if result.Message == "" {
			result.Message = fmt.Sprintf("%d checked; %d violations", result.Checked, result.Violations)
		}
		results = append(results, result)
	}
	return results
}

func indexAssertionDirectory(directory assertionDirectory) assertionDirectory {
	directory.peopleLookup = make(map[string]bool, len(directory.People))
	for _, person := range directory.People {
		directory.peopleLookup[person] = true
	}
	directory.memberCounts = make(map[string]int, len(directory.Groups))
	for name, members := range directory.Groups {
		distinct := make(map[string]bool, len(members))
		for _, member := range members {
			distinct[member] = true
		}
		directory.memberCounts[name] = len(distinct)
	}
	return directory
}
