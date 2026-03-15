---
name: cron-task
description: "Create workspace cron tasks that run the agent on a schedule. Use when the user wants a repeating background task, periodic inbox triage, scheduled reports, or any recurring agent workflow."
---

# Cron Task Skill

Use the `create_cron` tool to create scheduled agent tasks under the workspace `crons/` directory.

## When To Use

Use this skill when the user wants the agent to:

- check something on a schedule
- run a repeated workflow every N minutes/hours/days
- build a recurring inbox triage or report task
- save a prompt as a workspace cron job instead of running it once

Do not use this skill for one-shot work. For one-time execution, run the task directly.

## Scene Detection — Roleplay Cron

Before creating the task, check whether the cron involves the agent acting under a defined identity or persona. This includes ANY of the following:

- Acting as a specific character (fictional or otherwise)
- Operating as a professional role: programmer, manager, analyst, reviewer, support agent, moderator, etc.
- Maintaining a consistent personality, tone, or speaking style across runs
- Replying to messages or conversations in character
- Any task where "who the agent is" matters for behavior

If the task matches any of the above, call `get_skill("roleplay-cron")` and follow that skill's instructions for creating the cron. The roleplay-cron skill defines the correct file architecture (SOUL.md, SKILL.md, task.md separation) and anti-patterns to avoid. Do not proceed with the generic task skeleton below — use roleplay-cron instead.

If the task is purely mechanical (data fetching, report generation, cleanup, monitoring) with no identity requirements, continue with the generic flow below.

## Required Output Shape

When creating a cron task, you must produce a complete task definition, not a vague reminder.

Every cron task should explicitly include:

1. Which skill to read first, if the task depends on a workspace skill.
2. The main execution flow in order.
3. The decision rules for when to act, skip, or stop.
4. Any required environment details that must be used inside the task.
5. The expected reply or artifact behavior.

## Cron Expression Rules

- Use standard 5-field cron expressions: `minute hour day-of-month month day-of-week`
- Example every 5 minutes: `*/5 * * * *`
- Example every day at 09:30: `30 9 * * *`

## Task Writing Rules

Write the task as an executable instruction block for a future agent run.

The task should:

- be self-contained
- state the target skill by name
- require `get_skill` before using that skill's workflow
- tell the agent how to decide which items need action
- state what to do when there is nothing worth doing
- end with a clear stop condition

If the task involves replying to messages, require:

1. reading context first
2. an internal structured summary before replying
3. a short reply plan
4. then the actual send step

## Recommended Task Skeleton

```text
First call get_skill for <skill-name> and follow that skill strictly.

Execution flow:
1. ...
2. ...
3. ...

Decision rules:
1. reply when ...
2. skip when ...
3. stop when ...

If no action is needed, explicitly say so and stop.
```

## QQ Inbox Example

For a recurring QQ inbox task, write the task so it says all of the following:

1. First call `get_skill` for `qqinbox`.
2. Use the QQ inbox workflow from that skill.
3. Read recent conversations before replying.
4. Internally summarize topic, intent, and whether a reply is needed.
5. State a short reply plan.
6. Send replies only when justified by the conversation.
7. If there is nothing worth replying to, explicitly skip and stop.

## Tool Usage

Use `create_cron` with:

- `cron_id`: stable workspace cron identifier
- `cron_expression`: standard 5-field cron expression
- `task`: the full task definition text
- `enabled`: whether the cron should start enabled

## Important Constraint

Do not create under-specified cron tasks like:

- "check qq"
- "run every 5 minutes"
- "look and reply if needed"

Those are too vague for later scheduled execution.
