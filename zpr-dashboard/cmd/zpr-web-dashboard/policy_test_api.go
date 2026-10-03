package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxPolicyTestRequestBytes = (4 << 20) + maxPolicySourceBytes
	maxPolicyTestScriptBytes  = 8 << 20
	maxPolicyTestOutputBytes  = 32 << 20
	maxPolicyTestActors       = 500
	maxPolicyTestServices     = 100
	maxPolicyTestPairs        = 2000
	policyTestTimeout         = 90 * time.Second
)

var policyTestSourceLine = regexp.MustCompile(`^\s*\(line\s+([0-9]+)\)\s*(.*)$`)
var policyTestAttributeName = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

type policyTestRequest struct {
	Source   string                   `json:"source"`
	Actors   []policyTestActorInput   `json:"actors"`
	Services []policyTestServiceInput `json:"services"`
}

type policyTestActorInput struct {
	ID         string                     `json:"id"`
	Label      string                     `json:"label"`
	Kind       string                     `json:"kind"`
	Dimensions map[string]string          `json:"dimensions,omitempty"`
	Attributes []policyTestAttributeInput `json:"attributes"`
}

type policyTestAttributeInput struct {
	Key    string   `json:"key"`
	Values []string `json:"values"`
}

type policyTestServiceInput struct {
	ID         string                     `json:"id"`
	Name       string                     `json:"name"`
	Protocol   string                     `json:"protocol"`
	Port       int                        `json:"port"`
	ICMPType   int                        `json:"icmp_type,omitempty"`
	ICMPCode   int                        `json:"icmp_code,omitempty"`
	Attributes []policyTestAttributeInput `json:"attributes,omitempty"`
}

type policyTestSubject struct {
	ID         string            `json:"id"`
	Label      string            `json:"label"`
	Kind       string            `json:"kind"`
	Dimensions map[string]string `json:"dimensions,omitempty"`
}

type policyTestDecisionSummary struct {
	Count       int                 `json:"count"`
	ByKind      map[string]int      `json:"by_kind"`
	ByDimension map[string]int      `json:"by_dimension"`
	Subjects    []policyTestSubject `json:"subjects"`
}

type policyTestRuleResult struct {
	Indexes []int                     `json:"indexes"`
	Line    int                       `json:"line"`
	Source  string                    `json:"source"`
	Effect  string                    `json:"effect"`
	Matched policyTestDecisionSummary `json:"matched"`
}

type policyTestServiceResult struct {
	ID              string                    `json:"id"`
	Name            string                    `json:"name"`
	Protocol        string                    `json:"protocol"`
	Port            int                       `json:"port"`
	ICMPType        int                       `json:"icmp_type,omitempty"`
	ICMPCode        int                       `json:"icmp_code,omitempty"`
	Supported       bool                      `json:"supported"`
	Reason          string                    `json:"reason,omitempty"`
	EvaluatedActors int                       `json:"evaluated_actors"`
	Allowed         policyTestDecisionSummary `json:"allowed"`
	Denied          policyTestDecisionSummary `json:"denied"`
	DefaultDenied   policyTestDecisionSummary `json:"default_denied"`
	Rules           []policyTestRuleResult    `json:"rules"`
}

type policyTestResult struct {
	APIVersion   int                       `json:"api_version"`
	SourceSHA256 string                    `json:"source_sha256"`
	TestedAt     time.Time                 `json:"tested_at"`
	ActorCount   int                       `json:"actor_count"`
	Services     []policyTestServiceResult `json:"services"`
}

type policyTestCompiledRule struct {
	Index     int    `json:"index"`
	Source    string `json:"source"`
	ServiceID string `json:"service_id"`
	Allow     bool   `json:"allow"`
}

type policyTestHit struct {
	MatchIndex int    `json:"match_idx"`
	ZPLSource  string `json:"zpl_source"`
}

type policyTestZPTMessage struct {
	Kind        string                   `json:"kind"`
	Rules       []policyTestCompiledRule `json:"rules,omitempty"`
	Instruction int                      `json:"instruction,omitempty"`
	Decision    string                   `json:"decision,omitempty"`
	Hit         *policyTestHit           `json:"hit,omitempty"`
	Error       string                   `json:"error,omitempty"`
}

type policyTestEvalTarget struct {
	serviceID string
	actor     int
}

type policyTestRuleKey struct {
	serviceID string
	line      int
	source    string
	effect    string
}

type policyTestRuleAccumulator struct {
	result  policyTestRuleResult
	matched map[string]policyTestSubject
}

type policyTestServiceAccumulator struct {
	result        policyTestServiceResult
	allowed       map[string]policyTestSubject
	denied        map[string]policyTestSubject
	defaultDenied map[string]policyTestSubject
}

type policyTestBuffer struct {
	bytes.Buffer
	limit int
}

func (buffer *policyTestBuffer) Write(value []byte) (int, error) {
	if buffer.Len()+len(value) > buffer.limit {
		return 0, errors.New("policy test output exceeds the size limit")
	}
	return buffer.Buffer.Write(value)
}

func policyTesterAvailable() bool {
	tester := strings.TrimSpace(os.Getenv("ZPR_ZPT_BIN"))
	if tester == "" {
		tester = "zpt"
	}
	_, err := exec.LookPath(tester)
	return err == nil
}

func (a *application) handlePolicyTest(w http.ResponseWriter, r *http.Request) {
	if !localEditorRequest(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPolicyTestRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request policyTestRequest
	if err := decoder.Decode(&request); err != nil {
		writePolicyError(w, http.StatusBadRequest, "Invalid policy test request.")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writePolicyError(w, http.StatusBadRequest, "Invalid policy test request.")
		return
	}
	if err := validatePolicyTestRequest(request); err != nil {
		writePolicyError(w, http.StatusBadRequest, err.Error())
		return
	}
	if a.policy == nil {
		writePolicyError(w, http.StatusServiceUnavailable, "Policy testing is unavailable.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), policyTestTimeout)
	defer cancel()
	result, err := a.policy.testCandidate(ctx, request)
	if err != nil {
		writePolicyError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

func validatePolicyTestRequest(request policyTestRequest) error {
	if strings.TrimSpace(request.Source) == "" || len(request.Source) > maxPolicySourceBytes {
		return errors.New("Policy source is required and must be at most 1 MiB.")
	}
	if len(request.Actors) == 0 || len(request.Actors) > maxPolicyTestActors {
		return fmt.Errorf("Policy tests require between 1 and %d actors.", maxPolicyTestActors)
	}
	if len(request.Services) == 0 || len(request.Services) > maxPolicyTestServices {
		return fmt.Errorf("Policy tests require between 1 and %d services.", maxPolicyTestServices)
	}
	if len(request.Actors)*len(request.Services) > maxPolicyTestPairs {
		return fmt.Errorf("Policy test matrix exceeds the %d actor-service pair limit.", maxPolicyTestPairs)
	}
	actorIDs := make(map[string]bool, len(request.Actors))
	for _, actor := range request.Actors {
		if !policyTestValueSafe(actor.ID) || strings.TrimSpace(actor.ID) == "" || len(actor.ID) > 200 || actorIDs[actor.ID] {
			return errors.New("Actor IDs must be unique, nonempty, safe, and at most 200 characters.")
		}
		if strings.TrimSpace(actor.Label) == "" || len(actor.Label) > 200 || !policyTestValueSafe(actor.Label) {
			return fmt.Errorf("Actor %q requires a safe display label of at most 200 characters.", actor.ID)
		}
		if strings.TrimSpace(actor.Kind) == "" || len(actor.Kind) > 80 || !policyTestValueSafe(actor.Kind) {
			return fmt.Errorf("Actor %q requires a safe kind label.", actor.ID)
		}
		if len(actor.Dimensions) > 16 {
			return fmt.Errorf("Actor %q has too many identity dimensions.", actor.ID)
		}
		for dimension, value := range actor.Dimensions {
			if !policyTestAttributeName.MatchString(dimension) || strings.TrimSpace(value) == "" || len(value) > 200 || !policyTestValueSafe(value) {
				return fmt.Errorf("Actor %q has an invalid identity dimension.", actor.ID)
			}
		}
		actorIDs[actor.ID] = true
		if err := validatePolicyTestAttributes(actor.Attributes); err != nil {
			return fmt.Errorf("Actor %q: %w", actor.ID, err)
		}
	}
	serviceIDs := make(map[string]bool, len(request.Services))
	for _, service := range request.Services {
		if !policyTestValueSafe(service.ID) || strings.TrimSpace(service.ID) == "" || len(service.ID) > 200 || serviceIDs[service.ID] {
			return errors.New("Service IDs must be unique, nonempty, safe, and at most 200 characters.")
		}
		if strings.TrimSpace(service.Name) == "" || len(service.Name) > 200 || !policyTestValueSafe(service.Name) {
			return fmt.Errorf("Service %q requires a safe display name.", service.ID)
		}
		protocol := strings.ToUpper(service.Protocol)
		switch protocol {
		case "TCP", "UDP":
			if service.Port < 1 || service.Port > 65535 {
				return fmt.Errorf("Service %q has an invalid port.", service.ID)
			}
		case "ICMP6":
			if service.ICMPType < 0 || service.ICMPType > 255 || service.ICMPCode < 0 || service.ICMPCode > 255 {
				return fmt.Errorf("Service %q has an invalid ICMPv6 type or code.", service.ID)
			}
		default:
			return fmt.Errorf("Service %q must use TCP, UDP, or ICMP6.", service.ID)
		}
		serviceIDs[service.ID] = true
		if err := validatePolicyTestAttributes(service.Attributes); err != nil {
			return fmt.Errorf("Service %q: %w", service.ID, err)
		}
	}
	return nil
}

func validatePolicyTestAttributes(attributes []policyTestAttributeInput) error {
	if len(attributes) > 64 {
		return errors.New("at most 64 attributes are allowed")
	}
	seen := make(map[string]bool, len(attributes))
	for _, attribute := range attributes {
		if !policyTestAttributeName.MatchString(attribute.Key) || seen[attribute.Key] {
			return errors.New("attribute keys must be valid and unique")
		}
		seen[attribute.Key] = true
		if len(attribute.Values) > 64 {
			return fmt.Errorf("attribute %q has too many values", attribute.Key)
		}
		if len(attribute.Values) == 0 && !isPolicyTestTag(attribute.Key) {
			return fmt.Errorf("attribute %q requires a value", attribute.Key)
		}
		for _, value := range attribute.Values {
			if !policyTestValueSafe(value) || len(value) > 2048 {
				return fmt.Errorf("attribute %q contains a value that cannot be represented safely", attribute.Key)
			}
		}
	}
	return nil
}

func isPolicyTestTag(key string) bool {
	return strings.Contains(key, ".zpr.tag.")
}

func policyTestValueSafe(value string) bool {
	return !strings.ContainsAny(value, "\r\n,{}") && !strings.ContainsRune(value, 0)
}

func (workspace *policyWorkspace) testCandidate(ctx context.Context, request policyTestRequest) (policyTestResult, error) {
	if workspace.compiler == "" || workspace.configPath == "" || workspace.stageSigningKeyPath == "" {
		return policyTestResult{}, errors.New("Policy candidate testing is not configured.")
	}
	tester := strings.TrimSpace(os.Getenv("ZPR_ZPT_BIN"))
	if tester == "" {
		tester = "zpt"
	}
	tester, err := exec.LookPath(tester)
	if err != nil {
		return policyTestResult{}, errors.New("ZPT evaluator is not available.")
	}
	temporaryDirectory, err := os.MkdirTemp("", "zpr-policy-test-")
	if err != nil {
		return policyTestResult{}, errors.New("Unable to prepare an isolated policy test workspace.")
	}
	defer os.RemoveAll(temporaryDirectory)
	if err := os.Chmod(temporaryDirectory, 0o700); err != nil {
		return policyTestResult{}, errors.New("Unable to secure the policy test workspace.")
	}
	if _, err := workspace.compilePolicyTestCandidate(ctx, temporaryDirectory, request.Source); err != nil {
		return policyTestResult{}, err
	}
	script, targets, err := buildPolicyTestScript(request.Actors, request.Services)
	if err != nil {
		return policyTestResult{}, err
	}
	scriptPath := filepath.Join(temporaryDirectory, "candidate.zpt")
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		return policyTestResult{}, errors.New("Unable to prepare policy test evaluations.")
	}
	messages, err := runPolicyTestScript(ctx, tester, scriptPath, temporaryDirectory)
	if err != nil {
		return policyTestResult{}, err
	}
	return summarizePolicyTest(request, targets, messages)
}

func (workspace *policyWorkspace) compilePolicyTestCandidate(ctx context.Context, directory, source string) (string, error) {
	configPath, err := resolvedRegularFile(workspace.configPath)
	if err != nil {
		return "", errors.New("Policy ZPLC configuration is unavailable.")
	}
	keyPath, err := resolvedRegularFile(workspace.stageSigningKeyPath)
	if err != nil {
		return "", errors.New("Policy signing key is unavailable.")
	}
	sourcePath := filepath.Join(directory, "candidate.zpl")
	candidatePath := filepath.Join(directory, "candidate.bin2")
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		return "", errors.New("Unable to prepare the candidate policy source.")
	}
	compileContext, cancel := context.WithTimeout(ctx, policyStageTimeout)
	defer cancel()
	command := exec.CommandContext(compileContext, workspace.compiler, "-c", configPath, "-k", keyPath, "-f", "v2", "-o", candidatePath, sourcePath)
	command.Dir = filepath.Dir(configPath)
	output := &limitedBuffer{limit: maxPolicyOutputBytes}
	command.Stdout, command.Stderr = output, output
	if err := command.Run(); err != nil {
		diagnostics := strings.TrimSpace(strings.ReplaceAll(output.String(), directory, "policy-test"))
		if diagnostics == "" {
			return "", errors.New("ZPLC could not compile the candidate policy with the active runtime configuration.")
		}
		return "", fmt.Errorf("Candidate policy compilation failed: %s", diagnostics)
	}
	info, err := os.Stat(candidatePath)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxStagedPolicyBundleBytes {
		return "", errors.New("ZPLC produced no valid candidate policy bundle.")
	}
	return candidatePath, nil
}

func buildPolicyTestScript(actors []policyTestActorInput, services []policyTestServiceInput) (string, []policyTestEvalTarget, error) {
	var script strings.Builder
	script.WriteString("load candidate.bin2\n")
	for actorIndex, actor := range actors {
		actorName := fmt.Sprintf("client%d", actorIndex)
		if err := writePolicyTestAttributes(&script, actorName, actor.Attributes); err != nil {
			return "", nil, fmt.Errorf("Actor %q: %w", actor.ID, err)
		}
	}
	targets := make([]policyTestEvalTarget, 0, len(actors)*len(services))
	for serviceIndex, service := range services {
		actorName := fmt.Sprintf("service%d", serviceIndex)
		serviceAttributes := append([]policyTestAttributeInput(nil), service.Attributes...)
		serviceAttributes = append(serviceAttributes, policyTestAttributeInput{Key: "zpr.services", Values: []string{service.ID}})
		if err := writePolicyTestAttributes(&script, actorName, serviceAttributes); err != nil {
			return "", nil, fmt.Errorf("Service %q: %w", service.ID, err)
		}
		for actorIndex := range actors {
			if strings.EqualFold(service.Protocol, "TCP") {
				fmt.Fprintf(&script, "eval tcp client%d.49152 > %s.%d [S]\n", actorIndex, actorName, service.Port)
			} else if strings.EqualFold(service.Protocol, "UDP") {
				fmt.Fprintf(&script, "eval udp client%d.49152 > %s.%d\n", actorIndex, actorName, service.Port)
			} else {
				fmt.Fprintf(&script, "eval icmp6 client%d > %s %d:%d\n", actorIndex, actorName, service.ICMPType, service.ICMPCode)
			}
			targets = append(targets, policyTestEvalTarget{serviceID: service.ID, actor: actorIndex})
		}
	}
	if script.Len() > maxPolicyTestScriptBytes {
		return "", nil, errors.New("Policy test matrix exceeds the input size limit.")
	}
	return script.String(), targets, nil
}

func summarizePolicyTest(request policyTestRequest, targets []policyTestEvalTarget, messages []policyTestZPTMessage) (policyTestResult, error) {
	result := policyTestResult{
		APIVersion: 1, SourceSHA256: hashPolicyTestSource(request.Source), TestedAt: time.Now().UTC(),
		ActorCount: len(request.Actors), Services: make([]policyTestServiceResult, 0, len(request.Services)),
	}
	serviceAccumulators := make(map[string]*policyTestServiceAccumulator, len(request.Services))
	for _, service := range request.Services {
		serviceAccumulators[service.ID] = &policyTestServiceAccumulator{
			result:  policyTestServiceResult{ID: service.ID, Name: service.Name, Protocol: strings.ToUpper(service.Protocol), Port: service.Port, ICMPType: service.ICMPType, ICMPCode: service.ICMPCode, Supported: true, EvaluatedActors: len(request.Actors), Rules: []policyTestRuleResult{}},
			allowed: make(map[string]policyTestSubject), denied: make(map[string]policyTestSubject), defaultDenied: make(map[string]policyTestSubject),
		}
	}
	ruleAccumulators := make(map[policyTestRuleKey]*policyTestRuleAccumulator)
	rulesByIndex := make(map[int]policyTestCompiledRule)
	for _, message := range messages {
		if message.Kind != "POLICY_RULES" {
			continue
		}
		for _, rule := range message.Rules {
			rulesByIndex[rule.Index] = rule
			if strings.HasPrefix(rule.Source, "(builtin)") {
				continue
			}
			line, displaySource := policyTestRuleLine(rule.Source)
			effect := "deny"
			if rule.Allow {
				effect = "allow"
			}
			key := policyTestRuleKey{serviceID: rule.ServiceID, line: line, source: displaySource, effect: effect}
			if ruleAccumulators[key] == nil {
				ruleAccumulators[key] = &policyTestRuleAccumulator{
					result:  policyTestRuleResult{Line: line, Source: displaySource, Effect: effect, Indexes: []int{}},
					matched: make(map[string]policyTestSubject),
				}
			}
			ruleAccumulators[key].result.Indexes = append(ruleAccumulators[key].result.Indexes, rule.Index)
			if serviceAccumulators[rule.ServiceID] == nil {
				serviceAccumulators[rule.ServiceID] = &policyTestServiceAccumulator{
					result:  policyTestServiceResult{ID: rule.ServiceID, Name: rule.ServiceID, Supported: false, Reason: "No test endpoint was supplied for this service.", Rules: []policyTestRuleResult{}},
					allowed: make(map[string]policyTestSubject), denied: make(map[string]policyTestSubject), defaultDenied: make(map[string]policyTestSubject),
				}
			}
		}
	}
	if len(rulesByIndex) == 0 {
		return policyTestResult{}, errors.New("ZPT did not return the candidate policy rule catalog.")
	}
	targetByInstruction := make(map[int]policyTestEvalTarget, len(targets))
	for index, target := range targets {
		targetByInstruction[index+1] = target
	}
	responded := make(map[int]bool, len(targets))
	for _, message := range messages {
		switch message.Kind {
		case "POLICY_RULES":
			continue
		case "ERROR":
			if strings.Contains(message.Error, "NeedsRoute not handled") {
				return policyTestResult{}, errors.New("Candidate has route-constrained rules; route-aware policy testing is not available yet.")
			}
			return policyTestResult{}, fmt.Errorf("ZPT evaluation failed: %s", message.Error)
		case "EVAL":
			target, exists := targetByInstruction[message.Instruction]
			if !exists || target.actor < 0 || target.actor >= len(request.Actors) {
				return policyTestResult{}, errors.New("ZPT returned a result for an unknown test case.")
			}
			responded[message.Instruction] = true
			service := serviceAccumulators[target.serviceID]
			if service == nil {
				return policyTestResult{}, errors.New("ZPT returned a result for an unknown service.")
			}
			subject := policyTestSubject{
				ID: request.Actors[target.actor].ID, Label: request.Actors[target.actor].Label,
				Kind: request.Actors[target.actor].Kind, Dimensions: request.Actors[target.actor].Dimensions,
			}
			key := subject.ID
			switch message.Decision {
			case "ALLOW":
				service.allowed[key] = subject
			case "DENY":
				service.denied[key] = subject
			case "NO_MATCH":
				service.denied[key] = subject
				service.defaultDenied[key] = subject
			default:
				return policyTestResult{}, errors.New("ZPT returned an unknown policy decision.")
			}
			if message.Hit == nil {
				continue
			}
			rule, exists := rulesByIndex[message.Hit.MatchIndex]
			if !exists {
				return policyTestResult{}, errors.New("ZPT returned a hit for an unknown policy rule.")
			}
			effect := "deny"
			if rule.Allow {
				effect = "allow"
			}
			line, displaySource := policyTestRuleLine(rule.Source)
			ruleKey := policyTestRuleKey{serviceID: rule.ServiceID, line: line, source: displaySource, effect: effect}
			if accumulator := ruleAccumulators[ruleKey]; accumulator != nil {
				accumulator.matched[key] = subject
			}
		default:
			return policyTestResult{}, fmt.Errorf("ZPT returned unexpected message kind %q.", message.Kind)
		}
	}
	for instruction := range targetByInstruction {
		if !responded[instruction] {
			return policyTestResult{}, errors.New("ZPT did not return a decision for every test case.")
		}
	}
	for key, accumulator := range ruleAccumulators {
		service := serviceAccumulators[key.serviceID]
		accumulator.result.Matched = summarizePolicyTestSubjects(accumulator.matched)
		service.result.Rules = append(service.result.Rules, accumulator.result)
	}
	for _, service := range serviceAccumulators {
		service.result.Allowed = summarizePolicyTestSubjects(service.allowed)
		service.result.Denied = summarizePolicyTestSubjects(service.denied)
		service.result.DefaultDenied = summarizePolicyTestSubjects(service.defaultDenied)
		sort.Slice(service.result.Rules, func(i, j int) bool {
			if service.result.Rules[i].Line != service.result.Rules[j].Line {
				return service.result.Rules[i].Line < service.result.Rules[j].Line
			}
			return service.result.Rules[i].Effect < service.result.Rules[j].Effect
		})
		result.Services = append(result.Services, service.result)
	}
	sort.Slice(result.Services, func(i, j int) bool { return result.Services[i].ID < result.Services[j].ID })
	return result, nil
}

func writePolicyTestAttributes(script *strings.Builder, actorName string, attributes []policyTestAttributeInput) error {
	for _, attribute := range attributes {
		if len(attribute.Values) == 0 {
			domain, suffix, ok := strings.Cut(attribute.Key, ".zpr.tag.")
			if !ok || (domain != "user" && domain != "device") || suffix == "" {
				return errors.New("valueless attributes must be user or device tags")
			}
			fmt.Fprintf(script, "set %s #%s.%s\n", actorName, domain, suffix)
			continue
		}
		if len(attribute.Values) == 1 {
			fmt.Fprintf(script, "set %s %s:%s\n", actorName, attribute.Key, attribute.Values[0])
		} else {
			fmt.Fprintf(script, "set %s %s:{%s}\n", actorName, attribute.Key, strings.Join(attribute.Values, ", "))
		}
	}
	return nil
}

func runPolicyTestScript(ctx context.Context, tester, scriptPath, directory string) ([]policyTestZPTMessage, error) {
	command := exec.CommandContext(ctx, tester, "--json", "-i", scriptPath)
	command.Dir = directory
	output := &policyTestBuffer{limit: maxPolicyTestOutputBytes}
	command.Stdout, command.Stderr = output, output
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(output.String())
		if strings.Contains(message, "NeedsRoute not handled") {
			return nil, errors.New("Candidate has route-constrained rules; route-aware policy testing is not available yet.")
		}
		if message == "" {
			return nil, errors.New("ZPT could not evaluate the candidate policy.")
		}
		return nil, fmt.Errorf("ZPT evaluation failed: %s", message)
	}
	messages := make([]policyTestZPTMessage, 0)
	scanner := bufio.NewScanner(bytes.NewReader(output.Bytes()))
	scanner.Buffer(make([]byte, 64<<10), maxPolicyTestOutputBytes)
	for scanner.Scan() {
		var message policyTestZPTMessage
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			return nil, errors.New("ZPT returned malformed JSON policy test output.")
		}
		messages = append(messages, message)
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.New("ZPT policy test output exceeds the line size limit.")
	}
	return messages, nil
}

func policyTestRuleLine(source string) (int, string) {
	match := policyTestSourceLine.FindStringSubmatch(source)
	if len(match) != 3 {
		return 0, strings.TrimSpace(source)
	}
	line, _ := strconv.Atoi(match[1])
	return line, strings.TrimSpace(match[2])
}

func summarizePolicyTestSubjects(subjects map[string]policyTestSubject) policyTestDecisionSummary {
	result := policyTestDecisionSummary{ByKind: make(map[string]int), ByDimension: make(map[string]int), Subjects: make([]policyTestSubject, 0, len(subjects))}
	dimensionValues := make(map[string]map[string]bool)
	for _, subject := range subjects {
		result.Subjects = append(result.Subjects, subject)
		result.ByKind[subject.Kind]++
		for dimension, value := range subject.Dimensions {
			if dimensionValues[dimension] == nil {
				dimensionValues[dimension] = make(map[string]bool)
			}
			dimensionValues[dimension][value] = true
		}
	}
	for dimension, values := range dimensionValues {
		result.ByDimension[dimension] = len(values)
	}
	sort.Slice(result.Subjects, func(i, j int) bool {
		if result.Subjects[i].Label != result.Subjects[j].Label {
			return result.Subjects[i].Label < result.Subjects[j].Label
		}
		return result.Subjects[i].ID < result.Subjects[j].ID
	})
	result.Count = len(result.Subjects)
	return result
}

func hashPolicyTestSource(source string) string {
	hash := sha256.Sum256([]byte(source))
	return hex.EncodeToString(hash[:])
}
