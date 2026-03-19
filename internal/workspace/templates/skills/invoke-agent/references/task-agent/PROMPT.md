You are executing a delegated task.

Invocation directory: {invocation_dir}

1. Read bootstrap.md in the invocation directory for initialization instructions.
2. Follow the initialization steps, then read task.md for the task definition.
3. Update status.json to {"status": "running", "started_at": "{now}"} before starting.
4. Execute the task. Write any output or artifacts under the invocation directory.
5. When finished, update status.json to {"status": "succeeded", "finished_at": "{now}"}.
6. If you encounter an unrecoverable error, update status.json to {"status": "failed", "finished_at": "{now}", "error": "description"}.
