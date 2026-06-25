# CR-trpc-agent-go Architecture

## Goal

Build an automated Go code review agent on top of `trpc-agent-go` that can:

- accept a unified diff, PR patch, file list, or local workspace change
- load a dedicated code-review Skill and its rule scripts
- apply governance checks before any sandbox execution
- run optional static checks in an isolated runtime
- produce structured findings and a human-readable report
- persist tasks, decisions, findings, artifacts, and metrics

## Design Principle

The project is an application built on the framework, not a framework fork. The repository should own:

- review workflow
- rule engine
- diff parsing
- persistence schema
- report generation
- fixtures and tests

`trpc-agent-go` is used for reusable primitives such as skill loading, workspace execution, host execution, code execution, session/memory/storage patterns, and telemetry hooks.

## System Flow

1. CLI loads input from `--diff-file`, `--repo-path`, or a fixture.
2. Diff parser extracts files, hunks, line mapping, and package hints.
3. Skill loader reads `skills/code-review/SKILL.md`, rules, and scripts.
4. Governance layer evaluates whether a command or script may run.
5. Sandbox executor runs approved checks in container or E2B runtime.
6. Rule engine merges static analysis, heuristics, and diff-local checks.
7. Deduplication removes repeated findings on the same file, line, and rule class.
8. Report builder produces `review_report.json` and `review_report.md`.
9. Storage writes task state, decisions, runs, findings, artifacts, and metrics.

## Core Components

### CLI

Responsible for argument parsing, mode selection, and orchestration.

Supported modes:

- `rule-only`
- `dry-run`
- `sandbox`
- `fake-model`

### Input Parser

Parses:

- unified diff text
- file path lists
- git workspace changes

Outputs normalized file and hunk metadata with candidate line numbers.

### Skill Layer

Contains a `code-review` Skill folder with:

- `SKILL.md`
- rule docs
- scripts
- usage notes

The skill provides review policy and executable helper scripts.

### Governance Layer

Intercepts high-risk commands before execution.

It records:

- allow
- deny
- ask
- needs_human_review

The sandbox executor may only receive commands approved by policy.

### Sandbox Executor

Executes checks in an isolated runtime.

Preferred production runtimes:

- container
- E2B/Cube-style sandbox

Local execution is a fallback only for development and tests.

Controls:

- timeout
- output size limit
- env whitelist
- artifact cap
- secret redaction

### Rule Engine

Implements Go-review rules focused on the repo domain:

- security risk
- goroutine or context leak
- resource close lifecycle
- error handling
- test coverage gaps
- secret leakage
- database transaction or connection lifecycle

At least four categories must be supported in the first version.

### Deduper

Prevents duplicate reporting of the same issue by normalizing:

- file
- line
- category
- rule_id

Low-confidence items become warnings or human-review items, not high-confidence findings.

### Storage

Persists:

- review task
- input digest
- sandbox run
- permission decision
- filter decision
- finding
- artifact
- final report
- metrics summary

SQLite is the default backend. The storage interface should stay backend-neutral.

### Reporting

Produces:

- structured JSON
- Markdown summary

Reports include:

- findings summary
- severity distribution
- governance blocks
- sandbox summary
- human-review items
- metrics summary
- actionable recommendations

## Safety Boundaries

- no unrestricted shell execution
- no sandbox execution without policy approval
- no secret material in logs, artifacts, or reports
- no unlimited output capture
- no unbounded runtime
- no silent sandbox failure

## First Milestone

The first milestone is a deterministic rule-only pipeline:

- parse diff
- run rules
- dedupe
- redact
- persist
- report

That milestone must work without any real model API key.
