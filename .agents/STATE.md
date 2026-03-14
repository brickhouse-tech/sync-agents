
---
trigger: always_on
---

# State

This is the state template for sync-agents. It is used to define the state of the agent and its environment. The state markdown file itself is a human readble source for the agent to read and understand its current situation. It can be used to track the progress of the agent, identify any issues or challenges it may be facing, and provide a clear overview of its goals and objectives. The state is an important tool for the agent to use in order to make informed decisions and take appropriate actions to achieve its goals. By maintaining an up-to-date state, the agent can continuously learn and evolve, becoming more effective and efficient in its tasks.

The goal is to improve the performance of the agent by providing it with a clear and structured representation of its current situation. This allows the agent to make informed decisions and take appropriate actions to achieve its goals. The state is also used to track the progress of the agent and identify any issues or areas for improvement. By maintaining an up-to-date state, the agent can continuously learn and evolve, becoming more effective and efficient in its xtasks.

## TRACKING Agent State

Every state tracking will begin with a header of `### YYYYMMDDHHMMSS STATE: <STATE_NAME or OBJECTIVE` followed by a description of the state, any relevant information about the agent's performance, issues, or updates, and any actions taken or planned to address the current state. This format allows for easy tracking and monitoring of the agent's progress over time, as well as providing a clear record of the agent's history and development. By maintaining a detailed and organized state history, the agent can learn from past experiences and make informed decisions to improve its performance in the future.> 

The format above will be written below the `## STATE HISTORY BELOW` header in the STATE.md file. This allows for easy tracking and monitoring of the agent's progress over time, as well as providing a clear record of the agent's history and development. By maintaining a detailed and organized state history, the agent can learn from past experiences and make informed decisions to improve its performance in the future.

## Formatted Agent State

The formatted agent state will be a structured representation of the agent's current situation, including its goals, objectives, performance metrics, and any relevant information about its environment. This formatted state will be used by the agent to make informed decisions and take appropriate actions to achieve its goals. By maintaining an up-to-date and well-structured formatted state, the agent can continuously learn and evolve, becoming more effective and efficient in its tasks. The formatted state will also be used to track the progress of the agent and identify any issues or areas for improvement, allowing for continuous optimization of the agent's performance.

Example:

The state is structured as follows:

```json
{
  "agent_name": "string",
  "goals": ["string"],
  "skills": ["string"],
  "workflows": ["string"],
  "issues": ["string"],
  "last_updated": "timestamp"
}
```
to `.agents/state.json`

## STATE HISTORY BELOW




