package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAssertionExpressionEvaluation(t *testing.T) {
	staff := assertionDirectory{People: []string{"alice", "bob"}, Groups: map[string][]string{"Operators": {"alice", "bob", "alice"}}, Attributes: []string{"salary", "title"}, PersonAttributes: map[string]map[string][]string{"alice": {"salary": {"50000"}, "title": {"Engineer"}}, "bob": {"salary": {"40000"}, "title": {"Reviewer"}}}}
	hr := assertionDirectory{People: []string{"alice", "bob"}, Groups: map[string][]string{"Operators": {"alice", "bob"}}, Attributes: []string{"salary", "bonus"}, PersonAttributes: map[string]map[string][]string{"alice": {"salary": {"50000"}, "bonus": {"10000"}}, "bob": {"salary": {"40000"}, "bonus": {"1000"}}}}
	directory := staff
	directory.Sources = map[string]assertionDirectory{"staff": staff, "hr": hr}
	for _, test := range []struct {
		source, status      string
		checked, violations int
	}{
		{`assert source("staff").group("Operators").members == source("hr").group("Operators").members;`, "pass", 1, 0},
		{`people from "staff" where source("hr").attribute("salary") + source("hr").attribute("bonus") >= 50000;`, "fail", 2, 1},
		{`people from "staff" where source("hr").attribute("salary") == source("staff").attribute("salary");`, "pass", 2, 0},
		{`assert 1 + 2 * 3 == 7 and (1 + 2) * 3 == 9;`, "pass", 1, 0},
		{`assert 1 == 1 OR 1 == 2 AND 3 == 4;`, "pass", 1, 0},
		{`assert (1 == 1 or 1 == 2) and 3 == 4;`, "fail", 1, 1},
		{`assert -2 * -3 == 6 and 0.1 + 0.2 == 0.3 and 1 / 3 * 3 == 1 and 7 % 3 == 1;`, "pass", 1, 0},
		{`assert 1 == 1 or source("missing").group("X").members == 0;`, "error", 0, 0},
		{`assert 1 / 0 == 0;`, "error", 0, 0},
		{`assert 1 % 0 == 0;`, "error", 0, 0},
		{`assert 1.5 % 1 == 0;`, "error", 0, 0},
		{`people from "staff" in "Operators" where source("staff").attribute("title") == "Engineer" or source("staff").attribute("title") == "Reviewer";`, "pass", 2, 0},
		{`assert source("hr").group("Missing").members > 0;`, "error", 0, 0},
		{`assert 9007199254740991 + 1 > 0;`, "error", 0, 0},
		{`assert source("hr").person("missing").attribute("salary") > 0;`, "error", 0, 0},
		{`assert source("hr").person("alice").attribute("title") == "Engineer";`, "error", 0, 0},
		{`group "Operators" from "hr" members == 2;`, "pass", 1, 0},
		{`people from "hr" attribute "salary" >= 40000;`, "pass", 2, 0},
	} {
		rules, err := parseAssertions(test.source)
		if err != nil {
			t.Fatalf("parse %s: %v", test.source, err)
		}
		result := evaluateAssertions(rules, directory)[0]
		if result.Status != test.status || result.Checked != test.checked || result.Violations != test.violations {
			t.Errorf("%s: got %+v", test.source, result)
		}
	}
	for _, values := range [][]string{nil, {""}, {"50000", "60000"}, {"not-a-number"}} {
		hr.PersonAttributes["alice"]["salary"] = values
		rules, err := parseAssertions(`people from "staff" where source("hr").attribute("salary") > 0;`)
		if err != nil {
			t.Fatal(err)
		}
		if result := evaluateAssertions(rules, directory)[0]; result.Status != "error" {
			t.Fatalf("invalid scalar values: %+v", result)
		}
	}
}

func TestFormatAssertionSourcePreservesMeaningAndComments(t *testing.T) {
	source := "// Keep this comment\ngroup   \"Operators; // literal\"   members>=2; people attribute \"title\" in [\"Engineer\",\"Reviewer\"]; assert source(\"ldap\").group(\"Operators\").members>=2;"
	formatted, err := formatAssertionSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(formatted, "// Keep this comment\n") || !strings.Contains(formatted, `"Operators; // literal"`) || !strings.Contains(formatted, "members >= 2;\n") {
		t.Fatalf("formatted source lost content or spacing: %q", formatted)
	}
	again, err := formatAssertionSource(formatted)
	if err != nil || again != formatted {
		t.Fatalf("formatter is not idempotent: %q, %v", again, err)
	}
	originalRules, _ := parseAssertions(source)
	formattedRules, err := parseAssertions(formatted)
	if err != nil || len(originalRules) != len(formattedRules) {
		t.Fatalf("format changed rule count: %v", err)
	}
	directory := assertionDirectory{Groups: map[string][]string{"Operators": {"alice", "bob"}, "Operators; // literal": {"alice", "bob"}}}
	originalResults := evaluateAssertions(originalRules, directory)
	formattedResults := evaluateAssertions(formattedRules, directory)
	for index := range originalRules {
		if originalResults[index].Status != formattedResults[index].Status {
			t.Fatalf("format changed evaluation of rule %d", index)
		}
	}
	if _, err := formatAssertionSource(`group "Operators" members >=;`); err == nil {
		t.Fatal("invalid source was formatted")
	}
}

func TestAssertionExpressionParsing(t *testing.T) {
	for _, source := range []string{
		`assert (source("staff").group("Operators").members + 2) * 3 >= source("hr").group("Operators").members and 1 < 2 or 3 == 4;`,
		`people from "staff" in "Operators" where source("hr").attribute("salary") / 12 > 5000 AND source("staff").attribute("title") == "Engineer";`,
		`assert source("hr").person("alice").attribute("salary") >= 100.50;`,
	} {
		rules, err := parseAssertions(source)
		if err != nil || len(rules) != 1 || rules[0].Expression == nil || rules[0].ExpressionText == "" {
			t.Fatalf("parse %q: rules=%v error=%v", source, rules, err)
		}
	}
	for _, source := range []string{`assert not (1 == 2);`, `assert !(1 == 2);`, `assert (1 + 2;`, `assert 1 + ;`, `assert source("hr").attribute("password") == "x";`, `assert 1 + 2;`, `assert 1 and 2;`, `assert "x" + 1 == 2;`, `assert 1e999999999 > 0;`, `assert source("hr").attribute("title") == "Engineer";`} {
		if _, err := parseAssertions(source); err == nil {
			t.Fatalf("accepted invalid expression: %s", source)
		}
	}
	for _, source := range []string{"assert " + strings.Repeat("(", 65) + "1 == 1" + strings.Repeat(")", 65) + ";", "assert " + strings.Repeat("1 + ", 1025) + "1 == 1;"} {
		if _, err := parseAssertions(source); err == nil {
			t.Fatal("accepted an expression beyond complexity limits")
		}
	}
}

func TestAssertionExamplesParseAndEvaluateIndependently(t *testing.T) {
	source := `// Data assertions, not ZPL permissions.
group "Operators" members >= 2;
each group members > 0;
people exactly_one ["Employees", "Contractors"];
people in "Employees" not_both ["Administrators", "Auditors"];
`
	rules, err := parseAssertions(source)
	if err != nil || len(rules) != 4 {
		t.Fatalf("parse: %v; %d rules", err, len(rules))
	}
	directory := assertionDirectory{People: []string{"alice", "bob", "carol"}, Groups: map[string][]string{
		"Operators": {"alice", "alice", "bob"}, "Employees": {"alice", "bob"}, "Contractors": {"carol"},
		"Administrators": {"alice"}, "Auditors": {"bob"},
	}}
	for _, result := range evaluateAssertions(rules, directory) {
		if result.Status != "pass" {
			t.Fatalf("unexpected result: %+v", result)
		}
	}
	directory.Groups["Contractors"] = []string{"alice", "carol"}
	directory.Groups["Auditors"] = []string{"alice", "bob"}
	results := evaluateAssertions(rules, directory)
	if results[2].Status != "fail" || results[2].Violations != 1 || results[3].Status != "fail" || results[3].Subjects[0] != "alice" {
		t.Fatalf("missing membership violations: %+v", results)
	}
}

func TestAssertionComparatorsAndMissingMembership(t *testing.T) {
	directory := assertionDirectory{People: []string{"alice", "bob", "unassigned"}, Groups: map[string][]string{"A": {"alice", "bob"}, "B": {}}}
	for _, test := range []struct{ operator, status string }{{">", "fail"}, {">=", "pass"}, {"<", "fail"}, {"<=", "pass"}, {"==", "pass"}, {"!=", "fail"}} {
		rules, err := parseAssertions(`group "A" members ` + test.operator + ` 2;`)
		if err != nil {
			t.Fatal(err)
		}
		if result := evaluateAssertions(rules, directory)[0]; result.Status != test.status {
			t.Fatalf("%s: %+v", test.operator, result)
		}
	}
	rules, err := parseAssertions(`people exactly_one ["A", "B"]; people not_both ["A", "B"];`)
	if err != nil {
		t.Fatal(err)
	}
	results := evaluateAssertions(rules, directory)
	if results[0].Violations != 1 || results[0].Subjects[0] != "unassigned" || results[1].Status != "pass" {
		t.Fatalf("zero-membership semantics: %+v", results)
	}
}

func TestAssertionAttributeRulesAndMissingValues(t *testing.T) {
	directory := assertionDirectory{
		People: []string{"alice", "bob"}, Groups: map[string][]string{"Staff": {"alice", "bob"}},
		Attributes:       []string{"mail", "title", "uidnumber", "description"},
		PersonAttributes: map[string]map[string][]string{"alice": {"mail": {"alice@example.test"}, "title": {"Engineer"}, "uidnumber": {"1001"}}, "bob": {"title": {"Engineer", "Reviewer"}, "uidnumber": {"1002"}}},
		GroupAttributes:  map[string]map[string][]string{"Staff": {"description": {"All employees"}}},
	}
	for _, test := range []struct{ source, status string }{
		{`people attribute "mail" present;`, "fail"},
		{`people attribute "mail" != "forbidden";`, "fail"},
		{`people attribute "title" == "Engineer";`, "fail"},
		{`people in "Staff" attribute "title" in ["Engineer", "Reviewer"];`, "pass"},
		{`people attribute "title" contains "Engineer";`, "pass"},
		{`people attribute "uidNumber" >= 1000;`, "pass"},
		{`people attribute "uidNumber" > 1001;`, "fail"},
		{`group "Staff" attribute "description" present;`, "pass"},
		{`each group attribute "mail" absent;`, "pass"},
		{`people attribute "unknown" absent;`, "error"},
		{`people in "Missing" attribute "mail" present;`, "error"},
	} {
		rules, err := parseAssertions(test.source)
		if err != nil {
			t.Fatalf("parse %s: %v", test.source, err)
		}
		result := evaluateAssertions(rules, directory)[0]
		if result.Status != test.status {
			t.Fatalf("%s: %+v", test.source, result)
		}
	}
	for _, source := range []string{`people attribute "userPassword" present;`, `people attribute "privateKey" absent;`, `people attribute "title" > "Engineer";`, `people attribute "title" in [];`, `people attribute "uidNumber" == 1.5;`} {
		if _, err := parseAssertions(source); err == nil {
			t.Fatalf("accepted malformed/sensitive attribute rule %s", source)
		}
	}
	directory.PersonAttributes["alice"]["uidnumber"] = []string{"1", "invalid"}
	rules, _ := parseAssertions(`people attribute "uidNumber" > 10;`)
	if result := evaluateAssertions(rules, directory)[0]; result.Status != "error" {
		t.Fatalf("invalid numeric data must be an error: %+v", result)
	}
}

func TestAssertionLintWarningsAreAdvisory(t *testing.T) {
	source := "group \"Operators\" members >= 0;\npeople attribute \"mail\" present;\npeople attribute \"mail\" present;\nassert " + strings.Repeat("1 + ", 13) + "1 > 0;"
	rules, err := parseAssertions(source)
	if err != nil {
		t.Fatal(err)
	}
	warnings := lintAssertions(rules)
	codes := make(map[string]bool)
	for _, warning := range warnings {
		if warning.Line < 1 || warning.Severity != "warning" {
			t.Fatalf("invalid warning: %+v", warning)
		}
		codes[warning.Code] = true
	}
	for _, code := range []string{"ASSERT_TRIVIAL", "ASSERT_HUMAN_SCOPE", "ASSERT_DUPLICATE", "ASSERT_COMPLEXITY"} {
		if !codes[code] {
			t.Errorf("missing lint warning %s", code)
		}
	}
	clean, err := parseAssertions(`people in "Operators" attribute "mail" present; group "Operators" members >= 2;`)
	if err != nil || len(lintAssertions(clean)) != 0 {
		t.Fatalf("ordinary group assertions should not warn: %v", err)
	}
	numeric, err := parseAssertions(`people in "Operators" attribute "uidNumber" == 10; people in "Operators" attribute "uidNumber" == 10; people in "Operators" attribute "uidNumber" == 11;`)
	if err != nil {
		t.Fatal(err)
	}
	contradictions := 0
	for _, warning := range lintAssertions(numeric) {
		if warning.Code == "ASSERT_CONTRADICTION" {
			contradictions++
		}
	}
	if contradictions != 1 {
		t.Fatalf("numeric equality contradictions = %d, want one", contradictions)
	}
}

func TestAssertionLDAPAttributesAreApprovedBoundedAndPrivate(t *testing.T) {
	approved, err := approvedAssertionAttributes("mail,title,uidNumber,description,MAIL")
	if err != nil || len(approved) != 4 {
		t.Fatalf("attribute allowlist: %v %v", approved, err)
	}
	for _, names := range []string{"userPassword", "mail,accessToken", "privateKey", "*", "mail;range=0-1"} {
		if _, err := approvedAssertionAttributes(names); err == nil {
			t.Fatalf("accepted unsafe allowlist %q", names)
		}
	}
	input := "dn: uid=alice,dc=test\nobjectClass: inetOrgPerson\nuid: alice\ncn: Alice\nmail: alice@example.test\ntitle: Engineer\ntitle: Reviewer\nuidNumber: 1001\nuserPassword: private-value\n\n" +
		"dn: cn=Staff,dc=test\nobjectClass: posixGroup\ncn: Staff\nmemberUid: alice\ndescription: Employees\n"
	directory, err := parseAssertionLDAPAttributes(input, approved)
	if err != nil || len(directory.PersonAttributes["alice"]["title"]) != 2 || directory.GroupAttributes["Staff"]["description"][0] != "Employees" {
		t.Fatalf("attribute snapshot: %+v %v", directory, err)
	}
	if _, exists := directory.PersonAttributes["alice"]["userpassword"]; exists {
		t.Fatal("unrequested credential retained in snapshot")
	}
	summary, _ := json.Marshal(assertionSourceSummary(directory, time.Now()))
	if strings.Contains(string(summary), "alice@example.test") || strings.Contains(string(summary), "private-value") || strings.Contains(string(summary), "Engineer") {
		t.Fatal("source catalog exposed attribute values")
	}
	for _, invalid := range []string{
		strings.Replace(input, "title: Engineer", "title;range=0-0: Engineer", 1),
		strings.Replace(input, "mail: alice@example.test", "mail: "+strings.Repeat("x", 4097), 1),
	} {
		if _, err := parseAssertionLDAPAttributes(invalid, approved); err == nil {
			t.Fatal("partial/unbounded attribute snapshot accepted")
		}
	}
}

func TestAssertionsRejectMalformedAndPermissionSyntax(t *testing.T) {
	for _, source := range []string{
		`allow users to access services.`, `group Operators members >= 2;`,
		`group "A" members >= -1;`, `group "A" members >= 1.5;`, `group "A" members = 2;`,
		`people exactly_one [];`, `people exactly_one ["A", "A"];`,
		`people not_both ["A"];`, `group "A" members > 2`, strings.Repeat("x", maxAssertionSource+1),
	} {
		if _, err := parseAssertions(source); err == nil {
			t.Fatalf("accepted invalid assertion: %q", source)
		}
	}
}

func TestAssertionsDoNotPassUnknownOrEmptyScopes(t *testing.T) {
	for _, source := range []string{`group "Missing" members >= 0;`, `people exactly_one ["A"];`, `each group members >= 0;`} {
		rules, err := parseAssertions(source)
		if err != nil {
			t.Fatal(err)
		}
		result := evaluateAssertions(rules, assertionDirectory{Groups: map[string][]string{"A": {}}})[0]
		if strings.HasPrefix(source, "each") {
			result = evaluateAssertions(rules, assertionDirectory{Groups: map[string][]string{}})[0]
		}
		if result.Status != "error" {
			t.Fatalf("empty/unknown scope passed: %+v", result)
		}
	}
}

func TestAssertionLDAPUsesRealMembershipAndDecodesLDIF(t *testing.T) {
	if !assertionLDIFURL.MatchString("mem\n ber:< file:///private-data") || !assertionLDIFURL.MatchString("member:\n < file:///private-data") {
		t.Fatal("folded LDIF URL declaration was not rejected before parsing")
	}
	input := "dn: uid=alice,dc=test\nobjectClass: inetOrgPerson\nuid: alice\ncn: Alice\n\n" +
		"dn: cn=Operators,dc=test\nobjectClass: posixGroup\ncn:: T3BlcmF0b3Jz\nmemberUid: alice\nmemberUid: alice\n\n" +
		"dn: cn=Reviewers,dc=test\nobjectClass: groupOfNames\ncn: Reviewers\nmember: uid=alice,\n dc=test\n"
	directory, err := parseAssertionLDAP(input)
	if err != nil || len(directory.People) != 1 || len(directory.Groups["Operators"]) != 1 || len(directory.Groups["Reviewers"]) != 1 {
		t.Fatalf("LDAP membership parse: %+v; %v", directory, err)
	}
	for _, invalid := range []string{
		"dn:< file:///etc/passwd\n", "not valid LDIF", "",
		strings.Replace(input, "memberUid: alice", "memberUid: missing", 1),
		strings.Replace(input, "memberUid: alice", "memberUid;range=0-1: alice", 1),
		strings.Replace(input, "member: uid=alice,\n dc=test", "member: cn=Operators,dc=test", 1),
		input + "\ndn: cn=duplicate,dc=test\nobjectClass: posixGroup\ncn: Operators\n",
	} {
		if _, err := parseAssertionLDAP(invalid); err == nil {
			t.Fatalf("accepted invalid/incomplete LDAP input: %q", invalid)
		}
	}
}
