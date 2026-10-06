package main

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/scanner"
)

type assertionExpression struct {
	kind      string
	text      string
	source    string
	subject   string
	attribute string
	left      *assertionExpression
	right     *assertionExpression
}

func assertionPrecedence(operator string) int {
	switch strings.ToLower(operator) {
	case "or":
		return 1
	case "and":
		return 2
	case "==", "!=", ">", ">=", "<", "<=":
		return 3
	case "+", "-":
		return 4
	case "*", "/", "%":
		return 5
	}
	return 0
}

func (parser *assertionParser) expression(minimum int) *assertionExpression {
	parser.expressionDepth++
	defer func() { parser.expressionDepth-- }()
	if parser.expressionDepth > 64 {
		parser.fail("expression nesting exceeds 64 levels")
		return nil
	}
	left := parser.expressionPrimary()
	for parser.err == nil {
		operator := strings.ToLower(parser.text)
		precedence := assertionPrecedence(operator)
		if precedence == 0 && (operator == "=" || operator == "!") {
			precedence = 3
		}
		if precedence < minimum {
			break
		}
		parser.next()
		if operator == "=" || operator == "!" || operator == ">" || operator == "<" {
			if parser.text == "=" {
				operator += "="
				parser.next()
			} else if operator == "=" || operator == "!" {
				parser.fail("expected a comparison operator; general not is unsupported")
				return nil
			}
		}
		right := parser.expression(precedence + 1)
		left = parser.expressionNode(&assertionExpression{kind: "binary", text: operator, left: left, right: right})
	}
	return left
}

func (parser *assertionParser) expressionNode(node *assertionExpression) *assertionExpression {
	parser.expressionNodes++
	if parser.expressionNodes > 2048 {
		parser.fail("assertions may contain at most 2048 expression nodes")
		return nil
	}
	return node
}

func (parser *assertionParser) expressionPrimary() *assertionExpression {
	if parser.err != nil {
		return nil
	}
	if parser.text == "(" {
		parser.next()
		node := parser.expression(1)
		parser.expect(")")
		return node
	}
	if parser.text == "-" || parser.text == "+" {
		operator := parser.text
		parser.next()
		return parser.expressionNode(&assertionExpression{kind: "unary", text: operator, left: parser.expression(6)})
	}
	if parser.token == scanner.Int || parser.token == scanner.Float {
		node := parser.expressionNode(&assertionExpression{kind: "number", text: parser.text})
		parser.next()
		return node
	}
	if parser.token == scanner.String {
		value, err := strconv.Unquote(parser.text)
		if err != nil || len(value) > 4096 {
			parser.fail("expression string exceeds limits")
			return nil
		}
		node := parser.expressionNode(&assertionExpression{kind: "string", text: value})
		parser.next()
		return node
	}
	if parser.text != "source" {
		parser.fail("expected a number, string, source reference, or parenthesized expression; general not is unsupported")
		return nil
	}
	parser.next()
	parser.expect("(")
	alias := parser.name()
	parser.expect(")")
	parser.expect(".")
	node := &assertionExpression{kind: parser.text, source: alias}
	parser.next()
	switch node.kind {
	case "group", "person":
		parser.expect("(")
		node.subject = parser.name()
		parser.expect(")")
		parser.expect(".")
		if node.kind == "group" && parser.text == "members" {
			node.kind = "members"
			parser.next()
			return parser.expressionNode(node)
		}
	case "attribute":
		node.kind = "current_attribute"
	default:
		parser.fail("expected group, person, or attribute after source")
		return nil
	}
	if node.kind != "current_attribute" {
		parser.expect("attribute")
	}
	parser.expect("(")
	node.attribute = strings.ToLower(parser.name())
	if !validLDAPAttributeName(node.attribute) || assertionSensitiveAttribute(node.attribute) {
		parser.fail("attribute name is invalid or excluded for security")
	}
	parser.expect(")")
	return parser.expressionNode(node)
}

type assertionExpressionValue struct {
	kind    string
	text    string
	number  *big.Rat
	boolean bool
}

func validateAssertionExpression(node *assertionExpression, allowCurrent bool) (string, error) {
	if node == nil {
		return "", errors.New("Invalid expression")
	}
	switch node.kind {
	case "number":
		_, err := parseAssertionNumber(node.text)
		return "number", err
	case "string":
		return "string", nil
	case "members":
		return "number", nil
	case "person", "group":
		return "attribute", nil
	case "current_attribute":
		if !allowCurrent {
			return "", errors.New("Current-person attributes require people where")
		}
		return "attribute", nil
	}
	left, err := validateAssertionExpression(node.left, allowCurrent)
	if err != nil {
		return "", err
	}
	if node.kind == "unary" {
		if left != "number" && left != "attribute" {
			return "", errors.New("Unary arithmetic requires a number")
		}
		return "number", nil
	}
	right, err := validateAssertionExpression(node.right, allowCurrent)
	if err != nil {
		return "", err
	}
	if node.text == "and" || node.text == "or" {
		if left != "boolean" || right != "boolean" {
			return "", errors.New("And and Or require boolean comparisons")
		}
		return "boolean", nil
	}
	if assertionPrecedence(node.text) >= 4 {
		if left != "number" && left != "attribute" || right != "number" && right != "attribute" {
			return "", errors.New("Arithmetic requires numeric operands")
		}
		return "number", nil
	}
	if left == "boolean" || right == "boolean" {
		return "", errors.New("Comparisons require numbers or strings")
	}
	if node.text != "==" && node.text != "!=" && (left == "string" || right == "string") {
		return "", errors.New("Ordered comparisons require numeric operands")
	}
	if left == "number" && right == "string" || left == "string" && right == "number" {
		return "", errors.New("Cannot compare numeric and string literals")
	}
	return "boolean", nil
}

var assertionNumericLiteral = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]{1,2})?$`)

func parseAssertionNumber(text string) (*big.Rat, error) {
	if len(text) > 128 || !assertionNumericLiteral.MatchString(text) {
		return nil, errors.New("Invalid or excessive numeric value")
	}
	number, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil, errors.New("Invalid numeric value")
	}
	return checkedAssertionNumber(number)
}

func assertionExpressionNumber(value assertionExpressionValue) (*big.Rat, error) {
	if value.kind == "number" {
		return value.number, nil
	}
	if value.kind != "attribute" {
		return nil, errors.New("Arithmetic and ordered comparisons require numeric values")
	}
	return parseAssertionNumber(value.text)
}

func checkedAssertionNumber(number *big.Rat) (*big.Rat, error) {
	if number.Num().BitLen() > 256 || number.Denom().BitLen() > 256 || new(big.Rat).Abs(number).Cmp(new(big.Rat).SetInt64(9007199254740991)) > 0 {
		return nil, errors.New("Arithmetic value exceeds numeric limits")
	}
	return number, nil
}

func assertionExpressionDirectory(directory assertionDirectory, alias string) (assertionDirectory, error) {
	if selected, ok := directory.Sources[alias]; ok {
		return selected, nil
	}
	if alias == "ldap" && directory.Sources == nil {
		return directory, nil
	}
	return assertionDirectory{}, fmt.Errorf("Unknown or unavailable trusted source: %s", alias)
}

func (node *assertionExpression) evaluate(directory assertionDirectory, subject string) (assertionExpressionValue, error) {
	if node == nil {
		return assertionExpressionValue{}, errors.New("Invalid assertion expression")
	}
	switch node.kind {
	case "number":
		number, err := parseAssertionNumber(node.text)
		return assertionExpressionValue{kind: "number", number: number}, err
	case "string":
		return assertionExpressionValue{kind: "string", text: node.text}, nil
	case "members", "group", "person", "current_attribute":
		selected, err := assertionExpressionDirectory(directory, node.source)
		if err != nil {
			return assertionExpressionValue{}, err
		}
		name := node.subject
		if node.kind == "members" {
			_, exists := selected.Groups[name]
			if !exists {
				return assertionExpressionValue{}, fmt.Errorf("Unknown group in source %s: %s", node.source, name)
			}
			return assertionExpressionValue{kind: "number", number: new(big.Rat).SetInt64(int64(selected.memberCounts[name]))}, nil
		}
		attributes := selected.PersonAttributes
		if node.kind == "group" {
			if _, exists := selected.Groups[name]; !exists {
				return assertionExpressionValue{}, fmt.Errorf("Unknown group in source %s: %s", node.source, name)
			}
			attributes = selected.GroupAttributes
		} else {
			if node.kind == "current_attribute" {
				name = subject
			}
			if !selected.peopleLookup[name] {
				return assertionExpressionValue{}, fmt.Errorf("Unknown person in source %s: %s", node.source, name)
			}
		}
		approved := false
		for _, attribute := range selected.Attributes {
			if strings.EqualFold(attribute, node.attribute) {
				approved = true
				break
			}
		}
		if !approved {
			return assertionExpressionValue{}, fmt.Errorf("Attribute is not available from the approved source %s: %s", node.source, node.attribute)
		}
		values := attributes[name][node.attribute]
		if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
			return assertionExpressionValue{}, fmt.Errorf("Expression requires one nonblank attribute value in source %s for %s: %s", node.source, name, node.attribute)
		}
		return assertionExpressionValue{kind: "attribute", text: values[0]}, nil
	case "unary":
		value, err := node.left.evaluate(directory, subject)
		if err != nil {
			return assertionExpressionValue{}, err
		}
		number, err := assertionExpressionNumber(value)
		if err != nil {
			return assertionExpressionValue{}, err
		}
		if node.text == "-" {
			number = new(big.Rat).Neg(number)
		}
		return assertionExpressionValue{kind: "number", number: number}, nil
	case "binary":
		left, err := node.left.evaluate(directory, subject)
		if err != nil {
			return assertionExpressionValue{}, err
		}
		right, err := node.right.evaluate(directory, subject)
		if err != nil {
			return assertionExpressionValue{}, err
		}
		if node.text == "and" || node.text == "or" {
			if left.kind != "boolean" || right.kind != "boolean" {
				return assertionExpressionValue{}, errors.New("And and Or require boolean comparisons")
			}
			return assertionExpressionValue{kind: "boolean", boolean: node.text == "and" && left.boolean && right.boolean || node.text == "or" && (left.boolean || right.boolean)}, nil
		}
		if assertionPrecedence(node.text) >= 4 {
			leftNumber, err := assertionExpressionNumber(left)
			if err != nil {
				return assertionExpressionValue{}, err
			}
			rightNumber, err := assertionExpressionNumber(right)
			if err != nil {
				return assertionExpressionValue{}, err
			}
			number := new(big.Rat)
			switch node.text {
			case "+":
				number.Add(leftNumber, rightNumber)
			case "-":
				number.Sub(leftNumber, rightNumber)
			case "*":
				number.Mul(leftNumber, rightNumber)
			case "/":
				if rightNumber.Sign() == 0 {
					return assertionExpressionValue{}, errors.New("Division by zero")
				}
				number.Quo(leftNumber, rightNumber)
			case "%":
				if rightNumber.Sign() == 0 || !leftNumber.IsInt() || !rightNumber.IsInt() {
					return assertionExpressionValue{}, errors.New("Remainder requires integers and a nonzero divisor")
				}
				number.SetInt(new(big.Int).Rem(leftNumber.Num(), rightNumber.Num()))
			}
			number, err = checkedAssertionNumber(number)
			return assertionExpressionValue{kind: "number", number: number}, err
		}
		comparison := 0
		if left.kind == "number" || right.kind == "number" || node.text != "==" && node.text != "!=" {
			leftNumber, err := assertionExpressionNumber(left)
			if err != nil {
				return assertionExpressionValue{}, err
			}
			rightNumber, err := assertionExpressionNumber(right)
			if err != nil {
				return assertionExpressionValue{}, err
			}
			comparison = leftNumber.Cmp(rightNumber)
		} else {
			if left.kind == "boolean" || right.kind == "boolean" {
				return assertionExpressionValue{}, errors.New("Comparisons require numbers or strings")
			}
			comparison = strings.Compare(left.text, right.text)
		}
		valid := false
		switch node.text {
		case "==":
			valid = comparison == 0
		case "!=":
			valid = comparison != 0
		case ">":
			valid = comparison > 0
		case ">=":
			valid = comparison >= 0
		case "<":
			valid = comparison < 0
		case "<=":
			valid = comparison <= 0
		}
		return assertionExpressionValue{kind: "boolean", boolean: valid}, nil
	}
	return assertionExpressionValue{}, errors.New("Unknown expression kind")
}

func evaluateAssertionExpression(rule assertionRule, directory assertionDirectory) assertionResult {
	result := assertionResult{Rule: rule, Status: "pass", Subjects: []string{}}
	names := []string{""}
	if rule.Kind == "people_expression" {
		selected := directory
		if rule.Source != "" {
			var err error
			selected, err = assertionExpressionDirectory(directory, rule.Source)
			if err != nil {
				result.Status, result.Message = "error", err.Error()
				return result
			}
		}
		names = append([]string(nil), selected.People...)
		if rule.Scope != "" {
			members, exists := selected.Groups[rule.Scope]
			if !exists {
				result.Status, result.Message = "error", "Unknown group: "+rule.Scope
				return result
			}
			names = append([]string(nil), members...)
		}
		distinct := make(map[string]bool)
		for _, name := range names {
			distinct[name] = true
		}
		names = names[:0]
		for name := range distinct {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	for _, name := range names {
		value, err := rule.Expression.evaluate(directory, name)
		if err != nil {
			result.Status, result.Message = "error", err.Error()
			return result
		}
		if value.kind != "boolean" {
			result.Status, result.Message = "error", "Assertion expression must return a boolean comparison"
			return result
		}
		result.Checked++
		if !value.boolean {
			result.Status = "fail"
			result.Violations++
			if name != "" && len(result.Subjects) < 100 {
				result.Subjects = append(result.Subjects, name)
			}
		}
	}
	if result.Checked == 0 {
		result.Status, result.Message = "error", "No subjects found in the selected scope"
	} else {
		result.Message = fmt.Sprintf("%d checked; %d violations", result.Checked, result.Violations)
	}
	return result
}
