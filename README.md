# sync-agents

This is a shell based repository utilizing npm for easy installation and management. 

One set of agent rules to rule them all. Sync agents are a set of rules that can be applied to any agent to make it more effective. They are designed to be flexible and adaptable, allowing you to customize them to fit your specific needs.

The goal is to utilize AGENTS.md pointing to .agents as a source of truth for agent rules, skills, worflflows, and best practices. By doing so, we can ensure that all agents are following the same guidelines and standards, which will lead to better performance and more consistent results.

Goal is to sync for .claude, .windsurf, and .codex via utilziing .agents directory as the root source.

## Installation

`npm install @brickhouse-tech/sync-agents` or 

globally with `npm install -g @brickhouse-tech/sync-agents`

## Topology:

AGENTS.md will point to .agents directory, which will contain all the rules, skills, workflows, and best practices for the agents. The structure of the .agents directory will be as follows. The AGENTS.md file will explicitly index the rules, skills, workflows, and best practices for each agent, making it easy to find and apply the relevant information. This structure will allow for easy maintenance and updates to the agents, as all the information will be centralized in one location.

```
.agents/
  ├── rules/
  │   ├── rule1.md
  │   ├── rule2.md
  │   └── ...
  ├── skills/
  │   ├── skill1.md
  │   ├── skill2.md
  │   └── ...
  ├── workflows/
  │   ├── workflow1.md
  │   ├── workflow2.md
  │   └── ...
  |── STATE.md
```

Syncing is symlinks from .agents to .claude, .windsurf, and .codex. This allows for easy updates and maintenance of the agents, as any changes made to the .agents directory will automatically be reflected in the individual agent directories.

For explcitness the AGENTS.md file will also be symlinked to CLAUDE.md.

## STATE.md

This file will contain the current state of the agents and any relevant information about their performance, issues, or updates. It will serve as a central location for tracking the progress and status of the agents, allowing for easy monitoring and management. The STATE.md file will be updated regularly to reflect any changes or developments in the agents, ensuring that all stakeholders have access to the most up-to-date information. This way the Agent can resume work easily after a failure or interruption, as it can refer to the STATE.md file to determine where it left off and what tasks still need to be completed.

## Usage

```bash
sync-agents --help
```


