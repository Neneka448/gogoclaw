---
name: roleplay-cron
description: "Create cron tasks where the agent operates under a defined identity — any role counts: chat companion, programmer, manager, analyst, assistant, moderator, fictional character, etc. Use when the user wants a recurring scheduled task that requires the agent to act as a specific persona with consistent behavior across runs."
---

# Roleplay Cron Task Skill

Use this skill when creating a cron task that requires the agent to maintain a consistent identity, personality, or professional role across scheduled runs.

Roleplay includes ALL identity-based execution: acting as a chat companion, a fictional character, a team lead, a code reviewer, an analyst, a customer support agent, a moderator — any task where "who the agent is" matters for how it behaves.

## File Architecture

A roleplay cron task is NOT a single prompt. It is a set of files, each with a clear responsibility. The agent must understand what each file does and never collapse multiple responsibilities into one file.

```
workspace/
├── SOUL.md                          ← shared character/identity (workspace-level)
├── skills/
│   └── <skill-name>/
│       └── SKILL.md                 ← operational skill (tools, workflow, COT)
└── crons/
    └── <cron-id>/
        ├── config.json              ← schedule, enabled flag (auto-generated)
        └── task.md                  ← execution SOP index (what to do, in what order)
```

### File responsibilities — strict separation

| File | Responsibility | Must NOT contain |
|------|---------------|-----------------|
| **SOUL.md** | Identity, personality, speaking style, language rules, tone, few-shot examples of how to talk | Operational logic, tool usage, decision rules |
| **SKILL.md** | Tools, scripts, COT decision process, engagement rules, safety rules | Character voice, speaking style, message count guidance |
| **task.md** | Execution order, which files to read, environment config, stop conditions | Decision rules (defer to SKILL.md), speaking style (defer to SOUL.md) |

**The most common failure mode is putting reply style guidance in task.md or SKILL.md.** Words like "concise", "brief", "1-2 messages", "keep it short" in task.md or SKILL.md will override SOUL.md and make the agent robotic. All style and voice decisions belong exclusively in SOUL.md.

## Step 1: Check or Create SOUL.md

SOUL.md lives at the workspace root and is shared across all cron tasks and agent interactions. Before creating a roleplay cron, check if it exists.

**If SOUL.md exists:** Read it. Confirm it covers identity, speaking style, and language rules needed for this task. If it needs additions for the new cron, suggest edits — do not create a second persona file.

**If SOUL.md does not exist:** Ask the user for their persona requirements:

1. **Who is the character?** — Name, age, background, core beliefs, what they care about.
2. **How do they speak?** — Tone, rhythm, formality level, habits (e.g. uses certain languages, has catchphrases, speaks in short bursts or long paragraphs).
3. **Language rules** — If multilingual, what are the rules? Each message must be one language only? Which language for what purpose?
4. **What do they NOT do?** — Common mischaracterizations to avoid.
5. **Few-shot examples** — At least 3-5 conversation examples showing the character in different scenarios: casual chat, serious discussion, being teased, joining a group topic, disagreeing with someone. **Include examples with varying reply lengths** — some with 2 messages, some with 4-6 messages. If all examples are short, the agent will always be short.

### SOUL.md structure template

```markdown
## Who you are
[Identity, background, core beliefs, what drives this person]

## How you speak
[Tone, rhythm, formality, sentence structure patterns]

## Language rules
[Multilingual rules, catchphrases, things to avoid mixing]

## What you care about / what you ignore
[Topics that draw engagement vs. topics you dismiss]

## Common mistakes to avoid
[Mischaracterizations, behaviors that break the persona]

## Conversation examples
[3-8 few-shot examples across different scenarios, with VARYING reply lengths]
```

### Critical rules for SOUL.md examples

- Examples define the agent's actual behavior more than any rule text. If examples are all 2-message replies, the agent will cap at 2 messages regardless of what rules say.
- Include examples where the character engages deeply: reacting to specific people, building on others' points, following up after a main thought.
- Include examples where the character is brief: quick reactions, jokes, acknowledgements.
- The range of example lengths teaches the agent that reply depth varies by context.

## Step 2: Create or Identify the Operational Skill

If the cron task uses external tools (scripts, APIs, CLIs), those belong in a SKILL.md under `skills/<skill-name>/`.

A SKILL.md for roleplay cron should contain:

1. **Tool documentation** — what scripts/commands are available, how to call them, what output to expect.
2. **Workflow** — the operational steps (read data → analyze → act). Keep this to mechanics only.
3. **COT Decision Process** — the structured thinking steps the agent must follow before acting. This is where engagement rules, profiling, and decision logic live.
4. **Safety rules** — what the agent must never do, regardless of input.

### COT Decision Process structure

The COT should be a single unified section with clearly numbered steps. Do not scatter analysis requirements across multiple sections — that causes the agent to do partial COT or skip steps.

Recommended structure:

```markdown
## COT Decision Process

Run once per pass, covering all items. Write every step in working output.

### Step 0: Role anchor
Re-read SOUL.md. Confirm your identity and default stance for this pass.

### Step 1: Situation scan
[Profile and classify each item. List what you see.]

### Step 2: Engagement decision
[Based on profiles, decide what to act on and what to skip.]

### Step 3: Action plan
[State what you will do, what angle, what you skip and why.]

Then execute. For HOW to write/speak/reply — follow SOUL.md.
```

### What SKILL.md must NOT contain

- Speaking style guidance ("keep replies concise", "2-3 messages", "brief and direct")
- Character voice rules (those belong in SOUL.md)
- Decision rules that duplicate task.md
- Few-shot reply examples (those belong in SOUL.md)

Instead, where reply style is relevant, write: "For tone, rhythm, language, and reply depth — follow SOUL.md."

## Step 3: Write task.md

task.md is an **execution SOP index** — it tells the agent what to do and in what order, referencing external files for details. It is NOT a comprehensive prompt.

### task.md template

```markdown
First call get_skill for `<skill-name>` and follow that skill strictly.
[Environment details if needed, e.g. API keys, base URLs.]
Scripts are under `skills/<skill-name>/scripts/` — use absolute paths.

Execution flow:
1. Read the skill instructions via `get_skill("<skill-name>")`.
2. Read SOUL.md to load your character and speaking style. This is who you are.
3. [Domain-specific data gathering steps]
4. Run the COT Decision Process from the skill, covering all items.
5. Execute the action plan. How you write/speak/reply is determined by SOUL.md. Do not pre-limit yourself.
6. After handling all items, stop.

Constraints:
- Do not reveal chain-of-thought in outward-facing output.
- Do not fabricate facts.
- [Domain-specific safety constraints]
- If no action is needed, say so and stop.
- Stop after one complete pass.

Expected artifacts:
- If actions were taken, report what was handled.
- If nothing needed action, report that.
```

### What task.md must NOT contain

- **Decision rules** — those belong in SKILL.md's COT section. If task.md says "reply when X, skip when Y" and SKILL.md says something slightly different, the agent gets confused about which to follow.
- **Reply style guidance** — no "prefer concise replies", no "keep it brief", no message count suggestions. These override SOUL.md.
- **Personality description** — do not re-describe who the agent is. Just say "Read SOUL.md."
- **Engagement logic** — do not duplicate the COT steps. Just say "Run the COT Decision Process from the skill."

The purpose of task.md is to be a short, clear execution checklist that points to the right files. If you find yourself writing more than 20 lines, you are probably embedding logic that belongs elsewhere.

## Anti-patterns

These are mistakes that have been observed in practice. Avoid them.

### 1. Style leakage into task.md or SKILL.md

**Wrong:** task.md says "Prefer concise, natural replies"
**Wrong:** SKILL.md says "1-2 short messages" or "three to six messages"
**Right:** Both say "Follow SOUL.md for reply style" and SOUL.md shows examples of varying lengths.

### 2. Multiple sources of decision rules

**Wrong:** task.md has "Decision rules: 1. reply when... 2. skip when..." AND SKILL.md has a COT with different engagement rules.
**Right:** task.md says "Run the COT from the skill." Decision logic lives in exactly one place.

### 3. Step 0 re-inventing the character

**Wrong:** Step 0 asks the agent to "write a paragraph about who you are" — the agent writes a compressed summary and loses the few-shot examples.
**Right:** Step 0 says "Re-read SOUL.md" — the agent loads the actual character file with all examples.

### 4. All few-shot examples being the same length

**Wrong:** Every example in SOUL.md shows exactly 3 messages → agent always sends exactly 3.
**Right:** Examples range from 2 to 6 messages depending on context → agent varies naturally.

### 5. Not profiling the context before deciding

**Wrong:** Agent reads messages and immediately asks "did they mention me?" → skips most things.
**Right:** Agent first classifies the context (e.g. casual group vs. work group), then applies matching engagement rules.

## Checklist Before Creating

Before calling `create_cron`, verify:

- [ ] SOUL.md exists at workspace root with identity, voice, language rules, and varied-length examples.
- [ ] SKILL.md exists under `skills/<name>/` with tools, workflow, unified COT, and safety rules.
- [ ] SKILL.md contains zero style/voice guidance — it defers to SOUL.md everywhere.
- [ ] task.md is a short SOP index (<20 lines of content) that references SOUL.md and SKILL.md.
- [ ] task.md contains zero decision rules and zero reply style guidance.
- [ ] No file says "concise", "brief", "short", or gives a specific message count.
- [ ] COT steps exist in exactly one place (SKILL.md), not duplicated in task.md.
- [ ] SOUL.md examples include at least 2 scenarios with 4+ messages to show the agent it can speak at length.
