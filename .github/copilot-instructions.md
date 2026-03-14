# Copilot Code Review Priorities

When reviewing code in this repository, optimize for signal, not coverage.

## Review threshold

- Only leave a comment when you find a plausible behavioral defect, operational risk, security issue, data consistency issue, or maintainability issue with clear production impact.
- Prefer no comment over low-value comments.
- Do not comment on naming, formatting, typos, minor refactors, dead code cleanup, or stylistic preferences unless they directly cause a real bug or materially increase operational risk.

## Severity calibration

- Treat these as high priority: data loss, duplicate execution, broken recovery, stuck processing, deadlocks, global throughput collapse, shutdown or startup races, persistence lifecycle gaps, security vulnerabilities, backward compatibility breaks, and unbounded resource growth.
- Treat issues as medium priority only when there is a concrete and realistic failure mode.
- Avoid low-priority comments unless the author explicitly asks for cleanup or broad maintainability review.

## Repository context

- This repository is a stateful agent runtime.
- The highest-risk regressions are message loss, duplicate processing, session corruption, incorrect ordering within a session, persistence and recovery gaps, startup and shutdown ordering bugs, backpressure failures, and unbounded goroutine or memory growth.
- Prefer cross-file reasoning over narrow line-level nits.
- Review changes in terms of the full lifecycle: produce, queue, process, persist, stop, restart, and recover.

## What to inspect first

- Concurrency and serialization guarantees.
- Blocking behavior, queueing, and head-of-line blocking.
- Shutdown, restart, crash recovery, and retry semantics.
- Persistence correctness, atomicity, and repairability after partial failure.
- Boundedness of queues, caches, goroutines, file descriptors, and memory.
- Compatibility with existing persisted data, config, and long-running deployments.

## Review style

- Prefer fewer, higher-confidence comments.
- Every comment should explain the concrete failure mode, the trigger condition, the user or operational impact, and the smallest safe fix direction.
- Suppress speculative comments unless the risk is substantial and the reasoning is concrete.
- If a concern is valid but not urgent, say explicitly that it is medium or low priority and why.
