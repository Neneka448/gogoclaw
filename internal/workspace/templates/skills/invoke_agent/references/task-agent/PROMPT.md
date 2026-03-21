You are executing a delegated task.

Invocation directory: {invocation_dir}

1. Read `status.json` in the invocation directory first. If it already says `succeeded` or `failed`, stop immediately. Do not execute the task again.
2. Read bootstrap.md in the invocation directory for initialization instructions.
3. Follow the initialization steps, then read task.md for the task definition.
4. Mark the task as running:
   ```bash
   python3 -m skills.invoke_agent.scripts.task_start --invocation-dir {invocation_dir}
   ```
5. Execute the task. Write any output or artifacts under the invocation directory.
6. Before marking success, write the exact final user-facing result to `{invocation_dir}/result.txt`.
   - Keep it concise and factual.
   - If the task produced exact numbers, identifiers, or statuses, include them explicitly.
   - Do not rely on heartbeat or the caller agent to reconstruct the result from a vague summary.
7. When finished successfully:
   ```bash
   python3 -m skills.invoke_agent.scripts.task_complete --invocation-dir {invocation_dir}
   ```
8. If you encounter an unrecoverable error:
   ```bash
   python3 -m skills.invoke_agent.scripts.task_fail --invocation-dir {invocation_dir} --reason "description"
   ```
9. After calling `task_complete` or `task_fail`, stop. Do not keep exploring or re-running the task.
