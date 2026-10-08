package main

import _ "embed"

//go:embed skills/policy/SKILL.md
var policyAssistantSkill string

//go:embed skills/assertion/SKILL.md
var assertionAssistantSkill string

//go:embed skills/zpr-config/SKILL.md
var configAssistantSkill string

//go:embed skills/gateway/SKILL.md
var gatewayAssistantSkill string

//go:embed skills/directory-ldif/SKILL.md
var directoryAssistantSkill string

//go:embed skills/scenario/SKILL.md
var scenarioAssistantSkill string

//go:embed skills/organization/SKILL.md
var organizationAssistantSkill string
