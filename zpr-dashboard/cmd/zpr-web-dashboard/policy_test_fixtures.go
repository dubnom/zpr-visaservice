//go:build !production

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
)

type policyTestFixtureResponse struct {
	Actors   []policyTestActorInput   `json:"actors"`
	Services []policyTestServiceInput `json:"services"`
	Warnings []string                 `json:"warnings,omitempty"`
	// OmittedAttributes lists directory attributes whose values the ZPT fixture format cannot carry.
	OmittedAttributes []string `json:"omitted_attributes,omitempty"`
}

type policyTestActorFixture struct {
	input      policyTestActorInput
	attributes map[string][]string
}

func (a *application) handlePolicyTestFixtures(w http.ResponseWriter, r *http.Request) {
	if !localEditorRequest(w, r) {
		return
	}
	if a.policy == nil {
		writePolicyError(w, http.StatusServiceUnavailable, "Policy test fixtures are unavailable.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), policyTestTimeout)
	defer cancel()
	fixtures, err := a.policy.policyTestFixtures(ctx)
	if err != nil {
		writePolicyError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, fixtures)
}

func (workspace *policyWorkspace) policyTestFixtures(ctx context.Context) (policyTestFixtureResponse, error) {
	organizationID := strings.TrimSpace(os.Getenv("ZPR_POLICY_ORGANIZATION_ID"))
	if organizationID == "" || workspace.ldapContainer == "" || workspace.ldapBaseDN == "" || workspace.ldapBindDN == "" {
		return policyTestFixtureResponse{}, errors.New("Test fixture provider is not configured.")
	}
	config, err := configuredPolicyFixtures(workspace.ldapBaseDN, workspace.attributeMappings)
	if err != nil {
		return policyTestFixtureResponse{}, err
	}
	attributes, err := policyTestLDAPAttributes(config.Directory, workspace.attributeMappings)
	if err != nil {
		return policyTestFixtureResponse{}, err
	}
	directory, err := readAssertionLDAP(ctx, workspace.ldapContainer, workspace.ldapBindDN, workspace.ldapBaseDN, attributes)
	if err != nil {
		return policyTestFixtureResponse{}, err
	}
	actors, omitted, warnings, err := buildPolicyTestActorFixtures(config, directory, workspace.attributeMappings)
	if err != nil {
		return policyTestFixtureResponse{}, err
	}
	services := config.Services
	warnings = append(warnings, config.Warnings...)
	if len(actors) == 0 {
		return policyTestFixtureResponse{}, errors.New("No user or device test fixtures are available in the current directory.")
	}
	if len(services) == 0 {
		return policyTestFixtureResponse{}, errors.New("No testable service fixtures are configured for this policy context.")
	}
	return policyTestFixtureResponse{Actors: actors, Services: services, Warnings: warnings, OmittedAttributes: omitted}, nil
}

func policyTestLDAPAttributes(directory policyFixtureDirectory, mappings []policyAttributeMapping) ([]string, error) {
	names := strings.Split(defaultAssertionAttributeNames, ",")
	names = append(names, directory.Attributes...)
	names = append(names, directory.UserAttributes...)
	for _, source := range directory.IdentityMappings {
		names = append(names, source)
	}
	for _, source := range directory.UserIdentityMappings {
		names = append(names, source)
	}
	for _, mapping := range mappings {
		names = append(names, mapping.Source)
	}
	return approvedAssertionAttributes(strings.Join(names, ","))
}

func buildPolicyTestActorFixtures(config policyFixtureConfig, directory assertionDirectory, mappings []policyAttributeMapping) ([]policyTestActorInput, []string, []string, error) {
	users := make(map[string]policyTestActorFixture)
	machines := make(map[string]policyFixtureDevice, len(config.Devices))
	knownMachines := make(map[string]bool, len(config.DeviceOwners))
	scope := make(map[string]bool, len(config.DeviceScope))
	for _, id := range config.DeviceScope {
		scope[id], knownMachines[id] = true, true
	}
	for id := range config.DeviceOwners {
		knownMachines[id] = true
	}
	for _, machine := range config.Devices {
		if len(scope) > 0 && !scope[machine.ID] {
			continue
		}
		machines[machine.ID] = machine
		knownMachines[machine.ID] = true
	}
	machineAttributes := make(map[string]map[string][]string)
	for uid, raw := range directory.PersonAttributes {
		if !isPolicyTestMachine(uid, raw, knownMachines) {
			continue
		}
		id := policyTestMachineID(uid, raw)
		if len(scope) > 0 && !scope[id] && !scope[uid] {
			continue
		}
		knownMachines[id] = true
		machineAttributes[id] = raw
		if _, exists := machines[id]; !exists {
			machines[id] = policyFixtureDevice{ID: id}
		}
	}
	for _, uid := range directory.People {
		raw := directory.PersonAttributes[uid]
		if isPolicyTestMachine(uid, raw, knownMachines) {
			continue
		}
		name := policyTestDisplayName(raw, config.UserLabels[uid])
		if name == "" {
			name = uid
		}
		attributes := policyTestUserAttributes(uid, raw, config, mappings, directory.Groups)
		users[uid] = policyTestActorFixture{
			input: policyTestActorInput{ID: uid, Label: name, Kind: "user", Dimensions: map[string]string{"user": uid}}, attributes: attributes,
		}
	}

	deviceFixtures := make(map[string]policyTestActorFixture, len(machines))
	for id, machine := range machines {
		attributes := policyTestDeviceAttributes(machine, machineAttributes[id], config.Directory, mappings)
		deviceFixtures[id] = policyTestActorFixture{
			input: policyTestActorInput{ID: id, Label: id, Kind: "device", Dimensions: map[string]string{"device": id}}, attributes: attributes,
		}
	}
	fixtures := make([]policyTestActorFixture, 0, len(users)+len(deviceFixtures)+len(config.DeviceOwners))
	userIDs := make([]string, 0, len(users))
	for uid := range users {
		userIDs = append(userIDs, uid)
	}
	sort.Strings(userIDs)
	for _, uid := range userIDs {
		fixtures = append(fixtures, users[uid])
	}
	deviceIDs := make([]string, 0, len(deviceFixtures))
	for id := range deviceFixtures {
		deviceIDs = append(deviceIDs, id)
	}
	sort.Strings(deviceIDs)
	for _, id := range deviceIDs {
		fixtures = append(fixtures, deviceFixtures[id])
	}
	for _, id := range deviceIDs {
		owners := config.DeviceOwners[id]
		for _, uid := range owners {
			user, hasUser := users[uid]
			device, hasDevice := deviceFixtures[id]
			if !hasUser || !hasDevice {
				continue
			}
			merged := clonePolicyTestAttributes(user.attributes)
			mergePolicyTestAttributes(merged, device.attributes)
			fixtures = append(fixtures, policyTestActorFixture{
				input: policyTestActorInput{
					ID: uid + "@" + id, Label: user.input.Label + " on " + id, Kind: "user_device",
					Dimensions: map[string]string{"user": uid, "device": id},
					Attributes: nil,
				},
				attributes: merged,
			})
		}
	}
	if len(fixtures) > maxPolicyTestActors {
		return nil, nil, nil, fmt.Errorf("Test fixture population exceeds the %d-actor limit.", maxPolicyTestActors)
	}
	warnings := []string{}
	actors := make([]policyTestActorInput, 0, len(fixtures))
	omittedSet := map[string]bool{}
	for _, fixture := range fixtures {
		attributes, omitted := policyTestAttributeList(fixture.attributes)
		for _, key := range omitted {
			omittedSet[key] = true
		}
		fixture.input.Attributes = attributes
		actors = append(actors, fixture.input)
	}
	omitted := make([]string, 0, len(omittedSet))
	for key := range omittedSet {
		omitted = append(omitted, key)
	}
	sort.Strings(omitted)
	for _, key := range omitted {
		warnings = append(warnings, fmt.Sprintf("Directory attribute %q was omitted from test fixtures: a value contains a comma, brace or control character, or exceeds 2048 bytes, which the ZPT fixture format cannot carry. Policies that reference it cannot be analyzed.", key))
	}
	return actors, omitted, warnings, nil
}

func policyTestUserAttributes(uid string, raw map[string][]string, config policyFixtureConfig, mappings []policyAttributeMapping, groups map[string][]string) map[string][]string {
	attributes := map[string][]string{"user.zpr.authority": {config.UserAuthority}}
	for _, name := range config.Directory.UserAttributes {
		setPolicyTestAttribute(attributes, "user."+name, raw[strings.ToLower(name)])
	}
	for target, source := range config.Directory.UserIdentityMappings {
		setPolicyTestAttribute(attributes, target, raw[strings.ToLower(source)])
	}
	for _, mapping := range mappings {
		if strings.HasPrefix(mapping.Attribute, "user.") {
			setPolicyTestAttribute(attributes, strings.TrimSuffix(mapping.Attribute, "{}"), raw[strings.ToLower(mapping.Source)])
		}
	}
	for groupName, members := range groups {
		for _, member := range members {
			if member == uid {
				for _, mapping := range mappings {
					if strings.EqualFold(mapping.Source, "role") && strings.HasPrefix(mapping.Attribute, "user.") {
						attribute := strings.TrimSuffix(mapping.Attribute, "{}")
						setPolicyTestAttribute(attributes, attribute, append(attributes[attribute], groupName))
					}
				}
				tag := policyTestTag(groupName)
				if tag != "" {
					attributes["user.zpr.tag."+tag] = []string{}
				}
				break
			}
		}
	}
	return attributes
}

func policyTestDeviceAttributes(device policyFixtureDevice, raw map[string][]string, directory policyFixtureDirectory, mappings []policyAttributeMapping) map[string][]string {
	attributes := clonePolicyTestAttributes(device.Attributes)
	for _, name := range directory.Attributes {
		setPolicyTestAttribute(attributes, "device."+name, raw[strings.ToLower(name)])
	}
	for target, source := range directory.IdentityMappings {
		setPolicyTestAttribute(attributes, target, raw[strings.ToLower(source)])
	}
	for _, mapping := range mappings {
		if strings.HasPrefix(mapping.Attribute, "device.") {
			setPolicyTestAttribute(attributes, mapping.Attribute, raw[strings.ToLower(mapping.Source)])
		}
	}
	return attributes
}

func setPolicyTestAttribute(attributes map[string][]string, name string, values []string) {
	if len(values) > 0 {
		attributes[name] = append([]string(nil), values...)
	}
}

// policyTestAttributeList returns ZPT-safe attributes plus the keys omitted because a value cannot be represented.
func policyTestAttributeList(attributes map[string][]string) ([]policyTestAttributeInput, []string) {
	keys := make([]string, 0, len(attributes))
	for key := range attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]policyTestAttributeInput, 0, len(keys))
	omitted := []string{}
	for _, key := range keys {
		values := append([]string(nil), attributes[key]...)
		supported := true
		for _, value := range values {
			if !policyTestValueSafe(value) || len(value) > 2048 {
				supported = false
				break
			}
		}
		if !supported {
			omitted = append(omitted, key)
			continue
		}
		result = append(result, policyTestAttributeInput{Key: key, Values: values})
	}
	return result, omitted
}

func policyTestTag(value string) string {
	var result strings.Builder
	for _, char := range strings.ToLower(value) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-' {
			result.WriteRune(char)
		} else if char == ' ' || char == '_' {
			result.WriteByte('-')
		}
	}
	return strings.Trim(result.String(), "-")
}

func policyTestMachineID(uid string, attributes map[string][]string) string {
	if id := firstPolicyTestValue(attributes["zprmachineid"]); id != "" {
		return id
	}
	return uid
}

func policyTestDisplayName(attributes map[string][]string, fallback string) string {
	if name := firstPolicyTestValue(attributes["cn"]); name != "" {
		return name
	}
	return fallback
}

func firstPolicyTestValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func isPolicyTestMachine(uid string, attributes map[string][]string, known map[string]bool) bool {
	if known[uid] {
		return true
	}
	for _, objectClass := range attributes["objectclass"] {
		if strings.EqualFold(objectClass, "zprMachine") || strings.EqualFold(objectClass, "device") {
			return true
		}
	}
	return false
}

func clonePolicyTestAttributes(source map[string][]string) map[string][]string {
	result := make(map[string][]string, len(source))
	for key, values := range source {
		result[key] = append([]string(nil), values...)
	}
	return result
}

func mergePolicyTestAttributes(destination, source map[string][]string) {
	for key, values := range source {
		if len(values) == 0 {
			destination[key] = []string{}
			continue
		}
		seen := make(map[string]bool, len(destination[key]))
		for _, value := range destination[key] {
			seen[value] = true
		}
		for _, value := range values {
			if !seen[value] {
				destination[key] = append(destination[key], value)
				seen[value] = true
			}
		}
	}
}
