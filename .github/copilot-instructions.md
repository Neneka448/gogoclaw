# Copilot Code Review Priorities

When reviewing code in this repository, optimize for signal, not coverage.

## Review threshold

- Only leave a comment when you find a plausible behavioral defect, operational risk, security issue, data consistency issue, or maintainability issue with clear production impact.
- Prefer no comment over low-value comments.
- Do not comment on naming, formatting, typos, minor refactors, dead code cleanup, or stylistic preferences unless they directly cause a real bug or materially increase operational risk.

## Severity calibration

- High priority means a supported-platform, high-confidence defect that can cause data loss, unrecoverable corruption, duplicate execution, deadlocks, global throughput collapse, broken shutdown or startup recovery, or a security boundary break reachable from remote or otherwise untrusted input.
- Do not label an issue as high if it depends on unsupported platforms, local-only CLI misuse, rare operator action, or unusually large historical state or traffic.
- Medium priority means a real issue with a concrete failure mode, but one that depends on local operator input, large data volume, repeated restarts, non-default deployment assumptions, or degraded latency or cost without correctness loss.
- Low priority means test-only issues, unsupported-platform compatibility, cleanup opportunities, dead code, style, naming, or typo comments.
- Avoid low-priority comments unless the author explicitly asks for cleanup or broad maintainability review.

## Platform scope

- This repository currently targets macOS and Linux.
- Do not file comments based only on Windows behavior unless the change explicitly claims Windows support or introduces cross-platform abstraction that is intended to work there too.

## Repository context

- This repository is a stateful agent runtime.
- The highest-risk regressions are message loss, duplicate processing, session corruption, incorrect ordering within a session, persistence and recovery gaps, startup and shutdown ordering bugs, backpressure failures, and unbounded goroutine or memory growth.
- Prefer cross-file reasoning over narrow line-level nits.
- Review changes in terms of the full lifecycle: produce, queue, process, persist, stop, restart, and recover.

## Runtime verification

- For changes under gateway, agent, session, memory, message_bus, channels, or vectorstore, trace the full runtime path before commenting; do not infer behavior from a local diff alone.
- Verify trigger conditions, guards, persisted state, and prior side effects before claiming a branch will execute or a user-visible message will appear.
- Treat tests as assertions of current behavior, not proof of intended behavior. If suggesting that a test is wrong, first confirm the production path actually reaches the asserted state.
- For message-flow changes, reason end to end through: request entry, runtime initialization, session loading, memory sync or ingest, outbound queue writes, queue draining or dispatch, and shutdown.
- Do not leave a review comment that depends on a path condition unless that condition is explicitly proven from the code.

## Comment gate for lifecycle code

- Before leaving a comment on lifecycle-sensitive code, confirm all of the following from the code:
- What exact event triggers this path.
- What earlier code can prevent this path from running.
- What session, persisted, or in-memory state affects the outcome.
- What concrete user-visible behavior changes as a result.
- Why the current code or test is incorrect under the actual runtime flow.
- If any of these answers are unclear, do not leave the comment.

## What to inspect first

- Concurrency and serialization guarantees.
- Blocking behavior, queueing, and head-of-line blocking.
- Shutdown, restart, crash recovery, and retry semantics.
- Persistence correctness, atomicity, and repairability after partial failure.
- Boundedness of queues, caches, goroutines, file descriptors, and memory.
- Compatibility with existing persisted data, config, and long-running deployments.

## Review style

- Prefer fewer, higher-confidence comments.
- Leave at most 2 comments unless the pull request contains multiple independent high-confidence defects.
- Every comment should explain the concrete failure mode, the trigger condition, the user or operational impact, and the smallest safe fix direction.
- Suppress speculative comments unless the risk is substantial and the reasoning is concrete.
- If a concern is valid but not urgent, say explicitly that it is medium or low priority and why.
- Do not repeat the same concern in later review rounds unless the new diff materially changes the failure mode or severity.
