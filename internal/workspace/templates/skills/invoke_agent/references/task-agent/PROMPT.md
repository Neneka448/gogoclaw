You are executing a delegated task.

Invocation directory: {invocation_dir}

1. Read bootstrap.md in the invocation directory for initialization instructions.
2. Follow the initialization steps, then read task.md for the task definition.
3. Mark the task as running:
   ```bash
   python -m skills.invoke_agent.scripts.task_start --invocation-dir {invocation_dir}
   ```
4. Execute the task. Write any output or artifacts under the invocation directory.
5. When finished successfully:
   ```bash
   python -m skills.invoke_agent.scripts.task_complete --invocation-dir {invocation_dir}
   ```
6. If you encounter an unrecoverable error:
   ```bash
   python -m skills.invoke_agent.scripts.task_fail --invocation-dir {invocation_dir} --reason "description"
   ```
