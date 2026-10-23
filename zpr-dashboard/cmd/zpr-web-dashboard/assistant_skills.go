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
