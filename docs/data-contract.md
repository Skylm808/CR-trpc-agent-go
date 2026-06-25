# Data Contract

## ReviewTask

Represents one review execution.

Fields:

- `task_id`
- `input_type`
- `input_ref`
- `input_digest`
- `repo_path`
- `status`
- `mode`
- `created_at`
- `started_at`
- `finished_at`

## ReviewInput

Normalized input for review.

Fields:

- `source_type`
- `diff_text`
- `file_paths`
- `workspace_path`
- `base_ref`
- `head_ref`
- `parsed_files`
- `parsed_hunks`

## ParsedFile

Fields:

- `path`
- `language`
- `package_name`
- `is_test_file`
- `change_type`

## ParsedHunk

Fields:

- `file`
- `old_start`
- `old_lines`
- `new_start`
- `new_lines`
- `context`
- `candidate_lines`

## PermissionDecision

Fields:

- `decision_id`
- `task_id`
- `command`
- `policy_name`
- `decision`
- `reason`
- `created_at`

Allowed values:

- `allow`
- `deny`
- `ask`
- `needs_human_review`

## SandboxRun

Fields:

- `run_id`
- `task_id`
- `runtime`
- `command`
- `args`
- `timeout_ms`
- `output_limit_bytes`
- `env_whitelist`
- `status`
- `exit_code`
- `stdout_digest`
- `stderr_digest`
- `artifact_count`
- `duration_ms`
- `created_at`
- `finished_at`

## Finding

Fields:

- `finding_id`
- `task_id`
- `severity`
- `category`
- `file`
- `line`
- `title`
- `evidence`
- `recommendation`
- `confidence`
- `source`
- `rule_id`
- `dedupe_key`
- `status`

Allowed severities:

- `info`
- `low`
- `medium`
- `high`
- `critical`

Allowed statuses:

- `finding`
- `warning`
- `needs_human_review`

## Artifact

Fields:

- `artifact_id`
- `task_id`
- `kind`
- `path`
- `digest`
- `size_bytes`
- `created_at`

## MetricsSummary

Fields:

- `task_id`
- `total_duration_ms`
- `sandbox_duration_ms`
- `tool_call_count`
- `permission_block_count`
- `finding_count`
- `severity_counts`
- `exception_counts`
- `redaction_count`

## ReviewReport

Fields:

- `task_id`
- `summary`
- `findings`
- `warnings`
- `human_review_items`
- `governance_summary`
- `sandbox_summary`
- `metrics`
- `artifacts`
- `conclusion`

## Persistence Rules

- Every task must have one canonical task row.
- Decisions, runs, findings, artifacts, and metrics must link back to the task.
- Findings must be queryable by task id.
- Reports must be reconstructable from stored rows.
- Sensitive literals must be redacted before persistence when they appear in logs or report text.
