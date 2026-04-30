package agent

var RuleTemplate = `
---
trigger: always_on
---

# ${NAME}
`

var SkillTemplate = `---
trigger: always_on
---

# ${NAME}

## Description

Brief description of what this skill enables.

## Usage

When to use this skill and how to invoke it.

## Examples

` + "```" + `
Example usage or invocation here.
` + "```" + `

## Notes

Any caveats, prerequisites, or related skills.
`

var WorkflowTemplate = `---
trigger: always_on
---

# ${NAME}

## Trigger

Describe when this workflow should be activated.

## Steps

1. Step one
2. Step two
3. Step three

## Conditions

- Pre-conditions that must be met
- Any guards or checks

## Output

What the workflow produces or changes when complete.
`

var StateTemplate = `---
trigger: always_on
---

# State

Track project progress, current objectives, and resumption context.
Update this file regularly so agents can pick up where they left off.

## Save location

../STATE_${CONTEXT_DESCRIPTION}_YYYYMMDDHHMMSS.md

A new file will be created each time during agent executions as a short summary of the current state, progress, blockers, and next steps. This allows for a historical record of the project's evolution and provides context for future agent executions. The timestamp in the filename helps to keep track of when each state was recorded, and the context description provides a quick reference to the specific project or objective being tracked. Lastly there should exist session information / session id so we can resume openclaw tui and or claude-code sessions with the relevant context and state information.

Save both if necessary, but the state file is more important for tracking progress and providing context for future executions. The session information can be useful for resuming interactive sessions, but the state file is essential for maintaining a record of the project's evolution and providing context for future agent executions.

## Format

### YYYYMMDDHHMMSS STATE: <objective>

Description of current state, progress, blockers, and next steps.

Be sure to indicate whats left and what done via - [ ] checkboxes in state file
`
