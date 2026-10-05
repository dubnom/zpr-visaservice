package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

type policyTestFixtureResponse struct {
	Actors   []policyTestActorInput   `json:"actors"`
	Services []policyTestServiceInput `json:"services"`
	Warnings []string                 `json:"warnings,omitempty"`
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
	manifest, organization, err := manifestForOrganization(organizationID)
	if err != nil || organization.Directory.BaseDN != workspace.ldapBaseDN {
		return policyTestFixtureResponse{}, errors.New("The active test fixture context is unavailable.")
	}
	attributes, err := policyTestLDAPAttributes(organization, workspace.attributeMappings)
	if err != nil {
		return policyTestFixtureResponse{}, err
	}
	directory, err := readAssertionLDAP(ctx, workspace.ldapContainer, workspace.ldapBindDN, workspace.ldapBaseDN, attributes)
	if err != nil {
		return policyTestFixtureResponse{}, err
	}
	actors, warnings, err := buildPolicyTestActorFixtures(organization, manifest, directory, workspace.attributeMappings)
	if err != nil {
		return policyTestFixtureResponse{}, err
	}
	services, serviceWarnings := buildPolicyTestServiceFixtures(organization)
	warnings = append(warnings, serviceWarnings...)
	if len(actors) == 0 {
		return policyTestFixtureResponse{}, errors.New("No user or device test fixtures are available in the current directory.")
	}
	if len(services) == 0 {
		return policyTestFixtureResponse{}, errors.New("No testable service fixtures are configured for this policy context.")
	}
	return policyTestFixtureResponse{Actors: actors, Services: services, Warnings: warnings}, nil
}

func policyTestLDAPAttributes(organization simulatorOrganization, mappings []policyAttributeMapping) ([]string, error) {
	names := strings.Split(defaultAssertionAttributeNames, ",")
	names = append(names, organization.Directory.Attributes...)
	names = append(names, organization.Directory.UserAttributes...)
	for _, mapping := range mappings {
		names = append(names, mapping.Source)
	}
	return approvedAssertionAttributes(strings.Join(names, ","))
}

func buildPolicyTestServiceFixtures(organization simulatorOrganization) ([]policyTestServiceInput, []string) {
	services := make([]policyTestServiceInput, 0, len(organization.Services))
	warnings := []string{}
	if len(organization.PolicyTestServices) > 0 {
		services = make([]policyTestServiceInput, 0, len(organization.PolicyTestServices))
		for _, profile := range organization.PolicyTestServices {
			service := policyTestServiceInput{
				ID: profile.ID, Name: profile.Name, Protocol: strings.ToUpper(profile.Protocol),
				Port: profile.Port, ICMPType: profile.ICMPType, ICMPCode: profile.ICMPCode,
			}
			service.Attributes = []policyTestAttributeInput{{Key: "device.zpr.adapter.cn", Values: []string{profile.ActorCN}}}
			services = append(services, service)
		}
		return services, warnings
	}
	for _, profile := range organization.Services {
		parts := strings.Split(strings.TrimSpace(profile.Endpoint), "/")
		if len(parts) < 3 {
			warnings = append(warnings, fmt.Sprintf("%s was omitted: endpoint has no transport and port.", profile.Name))
			continue
		}
		protocol := strings.ToUpper(parts[len(parts)-2])
		port := 0
		fmt.Sscanf(parts[len(parts)-1], "%d", &port)
		if (protocol != "TCP" && protocol != "UDP") || port < 1 || port > 65535 {
			warnings = append(warnings, fmt.Sprintf("%s was omitted: only TCP/UDP endpoints with valid ports can be tested.", profile.Name))
			continue
		}
		serviceID := profile.PolicyID
		if serviceID == "" {
			serviceID = profile.Name
		}
		service := policyTestServiceInput{ID: serviceID, Name: profile.Name, Protocol: protocol, Port: port}
		if profile.ActorCN != "" {
			service.Attributes = []policyTestAttributeInput{{Key: "device.zpr.adapter.cn", Values: []string{profile.ActorCN}}}
		}
		services = append(services, service)
	}
	return services, warnings
}

func buildPolicyTestActorFixtures(organization simulatorOrganization, manifest simulatorManifest, directory assertionDirectory, mappings []policyAttributeMapping) ([]policyTestActorInput, []string, error) {
	users := make(map[string]policyTestActorFixture)
	machines := make(map[string]simulatorMachine, len(manifest.Machines))
	knownMachines := make(map[string]bool, len(organization.MachineOwners))
	for id := range organization.MachineOwners {
		knownMachines[id] = true
	}
	for _, machine := range manifest.Machines {
		if len(knownMachines) > 0 && !knownMachines[machine.ID] {
			continue
		}
		machines[machine.ID] = machine
	}
	machineAttributes := make(map[string]map[string][]string)
	for uid, raw := range directory.PersonAttributes {
		if !isPolicyTestMachine(uid, raw, knownMachines) {
			continue
		}
		id := policyTestMachineID(uid, raw)
		if len(knownMachines) > 0 && !knownMachines[id] && !knownMachines[uid] {
			continue
		}
		knownMachines[id] = true
		machineAttributes[id] = raw
		if _, exists := machines[id]; !exists {
			machines[id] = simulatorMachine{ID: id, Owner: firstPolicyTestValue(raw["zprmachineowner"])}
		}
	}
	personNames := make(map[string]string, len(organization.Directory.People))
	for _, person := range organization.Directory.People {
		personNames[person.UID] = person.Name
	}
	for _, uid := range directory.People {
		raw := directory.PersonAttributes[uid]
		if isPolicyTestMachine(uid, raw, knownMachines) {
			continue
		}
		name := policyTestDisplayName(raw, personNames[uid])
		if name == "" {
			name = uid
		}
		attributes := policyTestUserAttributes(uid, raw, organization, mappings, directory.Groups)
		users[uid] = policyTestActorFixture{
			input: policyTestActorInput{ID: uid, Label: name, Kind: "user", Dimensions: map[string]string{"user": uid}}, attributes: attributes,
		}
	}

	deviceFixtures := make(map[string]policyTestActorFixture, len(machines))
	for id, machine := range machines {
		if owners := organization.MachineOwners[id]; len(owners) > 0 {
			machine.Owner = owners[0]
		}
		attributes := policyTestDeviceAttributes(id, machine, machineAttributes[id], organization, mappings)
		deviceFixtures[id] = policyTestActorFixture{
			input: policyTestActorInput{ID: id, Label: id, Kind: "device", Dimensions: map[string]string{"device": id}}, attributes: attributes,
		}
	}
	fixtures := make([]policyTestActorFixture, 0, len(users)+len(deviceFixtures)+len(organization.MachineOwners))
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
		owners := organization.MachineOwners[id]
		if len(owners) == 0 {
			owner := machines[id].Owner
			if owner != "" && owner != "it-pool" {
				owners = []string{owner}
			}
		}
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
		return nil, nil, fmt.Errorf("Test fixture population exceeds the %d-actor limit.", maxPolicyTestActors)
	}
	warnings := []string{}
	if len(organization.MachineOwners) == 0 {
		warnings = append(warnings, "Device ownership uses the simulation manifest because no organization owner map is configured.")
	}
	actors := make([]policyTestActorInput, 0, len(fixtures))
	for _, fixture := range fixtures {
		attributes, err := policyTestAttributeList(fixture.attributes)
		if err != nil {
			return nil, nil, err
		}
		fixture.input.Attributes = attributes
		actors = append(actors, fixture.input)
	}
	return actors, warnings, nil
}

func policyTestUserAttributes(uid string, raw map[string][]string, organization simulatorOrganization, mappings []policyAttributeMapping, groups map[string][]string) map[string][]string {
	attributes := map[string][]string{"user.zpr.authority": {"demo-identity"}}
	for _, name := range organization.Directory.UserAttributes {
		setPolicyTestAttribute(attributes, "user."+name, raw[strings.ToLower(name)])
	}
	for target, source := range organization.Directory.UserIdentityMappings {
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

func policyTestDeviceAttributes(id string, machine simulatorMachine, raw map[string][]string, organization simulatorOrganization, mappings []policyAttributeMapping) map[string][]string {
	attributes := map[string][]string{
		"device.zpr.authority": {"zpr-bootstrap"}, "device.zpr.adapter.cn": {id},
		"device.zprMachineId": {id}, "device.zprMachineType": {machine.Type},
		"device.zprMachineModel": {machine.Model}, "device.zprMachineLocation": {machine.Location},
		"device.zprMachineSecure": {strconv.FormatBool(machine.Secure)}, "device.zprMachineOwner": {machine.Owner},
	}
	for _, name := range organization.Directory.Attributes {
		setPolicyTestAttribute(attributes, "device."+name, raw[strings.ToLower(name)])
	}
	for target, source := range organization.Directory.IdentityMappings {
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

func policyTestAttributeList(attributes map[string][]string) ([]policyTestAttributeInput, error) {
	keys := make([]string, 0, len(attributes))
	for key := range attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]policyTestAttributeInput, 0, len(keys))
	for _, key := range keys {
		values := append([]string(nil), attributes[key]...)
		for _, value := range values {
			if !policyTestValueSafe(value) || len(value) > 2048 {
				return nil, errors.New("A directory attribute cannot be represented safely in the policy test request.")
			}
		}
		result = append(result, policyTestAttributeInput{Key: key, Values: values})
	}
	return result, nil
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
	if known[uid] || strings.HasPrefix(strings.ToLower(uid), "machine-") {
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
