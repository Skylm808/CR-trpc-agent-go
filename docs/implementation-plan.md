# Implementation Plan

## Phase 1: Skeleton

- create Go module and CLI entrypoint
- add repository layout
- add deterministic rule-only pipeline
- add fixture loading

## Phase 2: Parsing and Rules

- parse unified diff
- derive file and hunk metadata
- attach package hints for Go files
- implement rule checks for:
  - security risk
  - goroutine or context leak
  - resource lifecycle
  - error handling
  - test missing
  - secret leakage
  - DB lifecycle
- add deduplication and redaction

## Phase 3: Persistence

- define SQLite schema
- implement repository interfaces
- persist task, decision, run, artifact, finding, metrics, and report rows
- add lookup by task id

## Phase 4: Sandbox and Governance

- add permission policy wrapper
- add command allow/deny interception
- wire container or E2B executor
- keep local runtime only as fallback
- enforce timeout and output limits

## Phase 5: Skill Packaging

- add `skills/code-review/SKILL.md`
- add rules docs
- add scripts for checks and heuristics
- document usage and limitations

## Phase 6: Reports and Samples

- generate `review_report.json`
- generate `review_report.md`
- add 8 sample diffs
- add README with run instructions

## Phase 7: Verification

- unit test diff parsing
- unit test deduplication
- unit test secret redaction
- unit test persistence queries
- unit test sandbox failure handling
- run end-to-end fixture reviews

## Milestone Order

1. deterministic rule-only pipeline
2. persistence and reports
3. governance and sandbox
4. skill packaging
5. fixtures and verification

## Definition of Done

- all sample diffs produce reports
- findings are structured and deduped
- storage can query by task id
- sandbox failures do not crash the review
- no secret values leak into reports or storage
