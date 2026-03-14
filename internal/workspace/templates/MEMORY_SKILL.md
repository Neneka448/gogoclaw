---
name: memory
description: Guide for consuming experiential memories via the recall_memory tool
triggers:
  - encountering a task that might have been done before
  - debugging a recurring issue
  - making a decision that could benefit from past experience
  - facing an unfamiliar scenario where prior context might help
---

# Memory Recall Skill

You have access to an experiential memory system that stores structured records of past interactions (both short-term episodes and long-term consolidated patterns). Use this skill to decide whether and how to leverage past experience.

## When to Recall

Before diving into a task, use this Chain-of-Thought process:

1. **Categorize the task**: What kind of scenario is this? (e.g., configuration, debugging, deployment, code review, design discussion)
2. **Assess familiarity**: Does this feel like something that might have been encountered before? Are there keywords, tools, or patterns that suggest prior experience?
3. **Decide**: If the answer to #2 is "possibly yes", call `recall_memory` with a descriptive query. If this is clearly novel or trivial, skip recall.

## How to Recall

Call the `recall_memory` tool with a natural language query describing the current situation:

```
recall_memory({"query": "configuring Nginx reverse proxy for load balancing on production server"})
```

Be **specific** in your query. Include:
- The technology/tool involved
- The type of task (setup, debug, optimize, etc.)
- The environment or context

## How to Use Recalled Memories

Each memory entry contains structured fields:

| Field  | Meaning |
|--------|---------|
| who    | Who was involved |
| what   | What the task was about |
| when   | When it happened |
| where  | In what environment/context |
| why    | The motivation |
| how    | **Detailed step-by-step narrative** of what was done, including problems encountered and how they were resolved |
| result | Whether it succeeded, partially succeeded, or failed |

### Applying memories:

- **Successful memories (result=success)**: Treat the `how` field as a reference approach. Follow similar steps but adapt to current specifics.
- **Failed memories (result=failure)**: Treat these as warnings. Avoid the approach described in `how`, or address the failure points proactively.
- **Partial memories (result=partial)**: Use what worked, and plan around what didn't.
- **High ref_count memories**: These have been recalled frequently, indicating a common pattern. Give them higher weight.
- **Long-term (kind=long_term) memories**: These are consolidated from multiple episodes and represent proven patterns. They are more reliable than individual episodes.

## Important

- Do NOT recall memory for every single message. Only recall when you genuinely think past experience could help.
- Do NOT blindly copy past approaches. Adapt them to the current context.
- If recall returns no results, that's fine — proceed normally.
- Memories are living knowledge. They improve over time through consolidation.
