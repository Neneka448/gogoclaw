---
applyTo: "cmd/**/*.go,internal/agent/**/*.go,internal/channels/**/*.go,internal/gateway/**/*.go,internal/memory/**/*.go,internal/message_bus/**/*.go,internal/session/**/*.go,internal/vectorstore/**/*.go"
---

# Runtime Review Rules

When reviewing runtime, transport, session, memory, or persistence code in this repository:

- Assume message sources can be bursty, duplicated, delayed, retried, or partially delivered.
- Look for cross-session interference, especially designs where one slow or blocked session can delay unrelated sessions.
- Look for shutdown sequences that can drop in-flight work, close dependencies too early, or panic on late writes.
- Look for startup and recovery logic that can skip persisted state, replay work incorrectly, or create duplicates.
- Look for persistence changes that can leave partially written state without a repair path.
- Look for caches or per-session state that can grow without bound in long-running processes.
- Look for queueing strategies that hide backpressure until memory growth or global blocking occurs.
- Look for startup work that scales with total historical data and may become operationally expensive over time.
- Prefer comments only when the code creates a realistic correctness or operability problem.
- Do not leave style-only, naming-only, or typo-only comments in these paths.

When you raise a concern in these paths, prioritize:

- data loss or duplicate processing
- ordering violations within a session
- persistence lifecycle gaps across produce, process, store, and recover
- broken crash recovery or restart behavior
- global head-of-line blocking or starvation
- unbounded goroutine, queue, or memory growth
- silent backward compatibility breaks for existing stored data
