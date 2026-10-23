//go:build !production

package main

import _ "embed"

//go:embed skills/directory-ldif/SKILL.md
var directoryAssistantSkill string

//go:embed skills/scenario/SKILL.md
var scenarioAssistantSkill string

//go:embed skills/organization/SKILL.md
var organizationAssistantSkill string
