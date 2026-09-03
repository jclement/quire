// The starter templates: a working set for someone who runs engineering —
// the 1:1, the decision record, the project brief, the incident review, the
// weekly review — plus defaults that shape every new meeting, person, project
// and daily note. Installed only when asked from Settings; quire never drops
// files into a vault unprompted.
package service

import (
	"fmt"

	"github.com/jclement/quire/internal/vault"
)

// starterTemplates is path → content. Defaults are named for their type;
// the rest declare what they are for.
var starterTemplates = map[string]string{
	"templates/daily.md": `# {{date}}

## Focus

- 

## Log

- 

## Captured

- [ ] 
`,
	"templates/meeting.md": `---
description: A meeting with attendees, an agenda, and action items that become tasks.
---
# {{title}}

**Attendees:** 

## Agenda

- 

## Notes

- 

## Action items

- [ ] 
`,
	"templates/person.md": `---
description: A person page — role, how you work together, what to remember.
---
# {{title}}

**Role:** 
**Company:** 

## How we work

- 

## Notes

- 
`,
	"templates/project.md": `---
description: A project brief — the why, the scope, the milestones, the risks.
---
# {{title}}

## Goal

One sentence: what is different when this is done.

## Why now

- 

## Scope

**In:** 
**Out:** 

## Milestones

- [ ] 

## Risks

- 
`,
	"templates/one-on-one.md": `---
for: meeting
description: A 1:1 — their agenda first, then yours, then what you each owe.
tags: [one-on-one]
---
# {{title}}

## Their items

- 

## My items

- 

## Follow-ups

- [ ] 
`,
	"templates/decision.md": `---
for: note
description: A decision record — context, options considered, the call, and its consequences.
tags: [decision]
---
# {{title}}

**Status:** proposed
**Date:** {{date}}

## Context

What situation forced a decision.

## Options

1. 
2. 

## Decision

What we chose, and why this over the others.

## Consequences

What gets easier, what gets harder, what we will revisit.
`,
	"templates/incident.md": `---
for: note
description: An incident review — impact, timeline, root cause, and the actions that stop it recurring.
tags: [incident]
---
# {{title}}

**Date:** {{date}}
**Impact:** 

## Timeline

- {{time}} — 

## Root cause

## What went well

- 

## Actions

- [ ] 
`,
	"templates/weekly.md": `---
for: weekly
description: The week: what it is for, and what it turned out to be.
---
# {{title}}

## This week is for

- 

## Retro

- 
`,

	"templates/weekly-review.md": `---
for: note
description: A weekly review — what landed, what slipped, what next week is for.
tags: [weekly-review]
---
# {{title}}

## Landed

- 

## Slipped

- 

## Next week

- [ ] 
`,
}

// InstallStarterTemplates writes any starter template that does not already
// exist, returning the paths written. Existing files are never touched.
func (s *Service) InstallStarterTemplates() ([]string, error) {
	var written []string
	for path, content := range starterTemplates {
		if s.Vault.Exists(path) {
			continue
		}
		if _, err := s.Vault.Write(path, []byte(content), ""); err != nil {
			return written, fmt.Errorf("writing %s: %w", path, err)
		}
		if _, err := s.Index.IndexFile(path); err != nil {
			return written, err
		}
		written = append(written, path)
	}
	if written == nil {
		written = []string{}
	}
	return written, nil
}

// ensure the type exists for callers that only know the constant.
var _ = vault.TypeTemplate
