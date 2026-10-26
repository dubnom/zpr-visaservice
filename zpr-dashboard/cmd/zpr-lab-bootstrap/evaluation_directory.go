package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

// EVALUATION ONLY (D6a): an install may serve an operator-supplied directory
// (an organization's own LDIF under its own base DN) instead of the starter
// directory. It is operator configuration: nothing here reads the Simulator.
const (
	evaluationDefaultBaseDN = "dc=evaluation,dc=zpr"
	// evaluationDirectorySchemaFile is written and mounted only for operator
	// directories.
	evaluationDirectorySchemaFile = "zpr-evaluation.schema"
	evaluationDirectoryMaxFile    = 4 << 20
	evaluationDirectoryMaxTotal   = 16 << 20
	evaluationDirectoryMaxEntries = 100000
)

type evaluationDirectory struct {
	BaseDN    string   `json:"base_dn"`
	LDIFFiles []string `json:"ldif_files"`
}

// Base-DN components are deliberately plain because the DN is written
// verbatim into slapd.conf and generated LDIF.
var evaluationBaseDNComponent = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*=[A-Za-z0-9][A-Za-z0-9._-]*$`)

func validateEvaluationBaseDN(value string) error {
	components := strings.Split(value, ",")
	for _, component := range components {
		if !evaluationBaseDNComponent.MatchString(component) {
			return errors.New("-directory-base-dn must be plain attr=value components joined by commas (letters, digits, '.', '_', '-')")
		}
	}
	if _, err := ldap.ParseDN(value); err != nil {
		return errors.New("-directory-base-dn is not a valid DN")
	}
	return nil
}

func (directory *evaluationDirectory) validate() error {
	if directory == nil {
		return nil
	}
	if err := validateEvaluationBaseDN(directory.BaseDN); err != nil {
		return err
	}
	if len(directory.LDIFFiles) == 0 {
		return errors.New("-directory-base-dn requires at least one -directory-ldif")
	}
	seen := map[string]bool{}
	for _, name := range directory.LDIFFiles {
		if !filepath.IsAbs(name) || filepath.Clean(name) != name || seen[name] {
			return errors.New("-directory-ldif requires distinct canonical absolute paths")
		}
		seen[name] = true
	}
	return nil
}

func (config evaluationConfig) directoryBaseDN() string {
	if config.Directory != nil {
		return config.Directory.BaseDN
	}
	return evaluationDefaultBaseDN
}

func (config evaluationConfig) directoryReaderDN() string {
	return "cn=zpr-reader," + config.directoryBaseDN()
}

type evaluationLDIFEntry struct {
	dn   *ldap.DN
	text []byte
}

// parseEvaluationLDIF accepts content records only (no change records or
// base64 DNs) so the import stays reviewable.
func parseEvaluationLDIF(name string, data []byte) ([]evaluationLDIFEntry, error) {
	var entries []evaluationLDIFEntry
	var current [][]byte
	flush := func() error {
		if len(current) == 0 {
			return nil
		}
		first := string(current[0])
		if !strings.HasPrefix(first, "dn: ") {
			return fmt.Errorf("%s: every record must start with a plain 'dn: ' line", name)
		}
		dn, err := ldap.ParseDN(strings.TrimSpace(strings.TrimPrefix(first, "dn: ")))
		if err != nil {
			return fmt.Errorf("%s: invalid DN %q", name, first)
		}
		for _, line := range current[1:] {
			attribute, _, ok := bytes.Cut(line, []byte(":"))
			if line[0] == ' ' {
				continue
			}
			if !ok || len(attribute) == 0 || bytes.EqualFold(attribute, []byte("changetype")) || bytes.EqualFold(attribute, []byte("dn")) {
				return fmt.Errorf("%s: %s is not a plain content record", name, first)
			}
		}
		entries = append(entries, evaluationLDIFEntry{dn: dn, text: append(bytes.Join(current, []byte("\n")), '\n')})
		current = nil
		return nil
	}
	for index, line := range bytes.Split(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")), []byte("\n")) {
		switch {
		case len(bytes.TrimSpace(line)) == 0:
			if err := flush(); err != nil {
				return nil, err
			}
		case line[0] == '#':
		case index == 0 && bytes.HasPrefix(line, []byte("version:")):
		case line[0] == ' ' && len(current) == 0:
			return nil, fmt.Errorf("%s: continuation line outside a record", name)
		default:
			current = append(current, line)
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return entries, nil
}

func evaluationLDIFHasAttribute(entry evaluationLDIFEntry, attribute string) bool {
	for line := range bytes.SplitSeq(entry.text, []byte("\n")) {
		name, _, ok := bytes.Cut(line, []byte(":"))
		if ok && bytes.EqualFold(bytes.TrimSpace(name), []byte(attribute)) {
			return true
		}
	}
	return false
}

// renderEvaluationDirectoryLDIF imports the operator LDIF unchanged, except
// that entries carrying userPassword (organization bind accounts) are never
// imported; the install provides its own reader at cn=zpr-reader,<base>.
func renderEvaluationDirectoryLDIF(directory *evaluationDirectory, readerHash string) ([]byte, []string, error) {
	base, err := ldap.ParseDN(directory.BaseDN)
	if err != nil {
		return nil, nil, err
	}
	reader, _ := ldap.ParseDN("cn=zpr-reader," + directory.BaseDN)
	subject, _ := ldap.ParseDN("uid=evaluation-subject,ou=People," + directory.BaseDN)
	people, _ := ldap.ParseDN("ou=People," + directory.BaseDN)
	groups, _ := ldap.ParseDN("ou=Groups," + directory.BaseDN)
	var entries []evaluationLDIFEntry
	total := 0
	for _, name := range directory.LDIFFiles {
		info, err := os.Lstat(name)
		if err != nil || !info.Mode().IsRegular() || info.Size() > evaluationDirectoryMaxFile {
			return nil, nil, fmt.Errorf("-directory-ldif %s must be a regular file of at most %d bytes", name, evaluationDirectoryMaxFile)
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return nil, nil, fmt.Errorf("read -directory-ldif %s: %w", name, err)
		}
		if total += len(data); total > evaluationDirectoryMaxTotal {
			return nil, nil, errors.New("-directory-ldif files are too large in total")
		}
		parsed, err := parseEvaluationLDIF(name, data)
		if err != nil {
			return nil, nil, err
		}
		entries = append(entries, parsed...)
	}
	if len(entries) == 0 || len(entries) > evaluationDirectoryMaxEntries {
		return nil, nil, fmt.Errorf("-directory-ldif must hold between 1 and %d entries", evaluationDirectoryMaxEntries)
	}
	var skipped []*ldap.DN
	var skippedNames []string
	var kept []evaluationLDIFEntry
	hasBase, hasPeople, hasGroups := false, false, false
	for index, entry := range entries {
		if !entry.dn.EqualFold(base) && !base.AncestorOfFold(entry.dn) {
			return nil, nil, fmt.Errorf("directory entry %s is outside %s", entry.dn, directory.BaseDN)
		}
		if entry.dn.EqualFold(reader) || entry.dn.EqualFold(subject) {
			return nil, nil, fmt.Errorf("directory entry %s is reserved for the install", entry.dn)
		}
		for _, earlier := range entries[:index] {
			if earlier.dn.EqualFold(entry.dn) {
				return nil, nil, fmt.Errorf("duplicate directory entry %s", entry.dn)
			}
		}
		if evaluationLDIFHasAttribute(entry, "userPassword") {
			skipped, skippedNames = append(skipped, entry.dn), append(skippedNames, entry.dn.String())
			continue
		}
		hasBase = hasBase || entry.dn.EqualFold(base)
		hasPeople = hasPeople || entry.dn.EqualFold(people)
		hasGroups = hasGroups || entry.dn.EqualFold(groups)
		kept = append(kept, entry)
	}
	if !hasBase || !kept[0].dn.EqualFold(base) {
		return nil, nil, fmt.Errorf("the first imported directory entry must be the base entry %s", directory.BaseDN)
	}
	for _, entry := range kept {
		for _, parent := range skipped {
			if parent.AncestorOfFold(entry.dn) {
				return nil, nil, fmt.Errorf("directory entry %s is below skipped credential entry %s", entry.dn, parent)
			}
		}
	}
	var out bytes.Buffer
	for _, entry := range kept {
		out.Write(entry.text)
		out.WriteByte('\n')
	}
	fmt.Fprintf(&out, "dn: cn=zpr-reader,%s\nobjectClass: organizationalRole\nobjectClass: simpleSecurityObject\ncn: zpr-reader\nuserPassword: %s\n", directory.BaseDN, readerHash)
	if !hasPeople {
		fmt.Fprintf(&out, "\ndn: ou=People,%s\nobjectClass: organizationalUnit\nou: People\n", directory.BaseDN)
	}
	if !hasGroups {
		fmt.Fprintf(&out, "\ndn: ou=Groups,%s\nobjectClass: organizationalUnit\nou: Groups\n", directory.BaseDN)
	}
	return out.Bytes(), skippedNames, nil
}

// evaluationDirectorySchema defines the zprMachine auxiliary class used by
// organization machine entries. Its OIDs are under a UUID arc (2.25, ITU-T
// X.667), so no registered enterprise number is implied.
const evaluationDirectorySchema = `# EVALUATION ONLY: minimal schema for organization machine entries.
# OIDs use a UUID arc (2.25, ITU-T X.667); this is not a published schema.
objectidentifier zprEvaluation 2.25.229988153311848237332792930596901282605
objectidentifier zprEvaluationAttribute zprEvaluation:1
objectidentifier zprEvaluationClass zprEvaluation:2

attributetype ( zprEvaluationAttribute:1 NAME 'zprMachineId'
	EQUALITY caseIgnoreMatch SUBSTR caseIgnoreSubstringsMatch
	SYNTAX 1.3.6.1.4.1.1466.115.121.1.15{256} )
attributetype ( zprEvaluationAttribute:2 NAME 'zprMachineType'
	EQUALITY caseIgnoreMatch SUBSTR caseIgnoreSubstringsMatch
	SYNTAX 1.3.6.1.4.1.1466.115.121.1.15{256} )
attributetype ( zprEvaluationAttribute:3 NAME 'zprMachineModel'
	EQUALITY caseIgnoreMatch SUBSTR caseIgnoreSubstringsMatch
	SYNTAX 1.3.6.1.4.1.1466.115.121.1.15{256} )
attributetype ( zprEvaluationAttribute:4 NAME 'zprMachineLocation'
	EQUALITY caseIgnoreMatch SUBSTR caseIgnoreSubstringsMatch
	SYNTAX 1.3.6.1.4.1.1466.115.121.1.15{256} )
attributetype ( zprEvaluationAttribute:5 NAME 'zprMachineSecure'
	EQUALITY booleanMatch
	SYNTAX 1.3.6.1.4.1.1466.115.121.1.7 )
attributetype ( zprEvaluationAttribute:6 NAME 'zprMachineOwner'
	EQUALITY caseIgnoreMatch SUBSTR caseIgnoreSubstringsMatch
	SYNTAX 1.3.6.1.4.1.1466.115.121.1.15{256} )

objectclass ( zprEvaluationClass:1 NAME 'zprMachine'
	DESC 'EVALUATION ONLY ZPR machine attributes'
	SUP top AUXILIARY
	MAY ( zprMachineId $ zprMachineType $ zprMachineModel $ zprMachineLocation $ zprMachineSecure $ zprMachineOwner ) )
`
