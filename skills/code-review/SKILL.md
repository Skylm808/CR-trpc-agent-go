# code-review

This skill provides the deterministic rule set and execution notes for the Go code review agent.

## Usage

- Load this skill before review execution.
- Prefer rule-only review for fixture tests.
- Use sandbox execution only after permission checks.

## Rules

- security risks
- goroutine and context leaks
- resource lifecycle issues
- error handling issues
- missing tests
- secret leakage
- database transaction and connection lifecycle issues

