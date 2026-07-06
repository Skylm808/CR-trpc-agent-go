# Review Fixtures

These diff fixtures exercise the first-version deterministic review path.

- `safe.diff`: clean Go change
- `secret.diff`: potential secret leakage
- `secret-shapes.diff`: common API key, LLM key, bearer, GitHub token, and placeholder cases
- `panic.diff`: direct panic path
- `todo.diff`: TODO marker
- `test-missing.diff`: missing-test warning
- `goroutine.diff`: goroutine-oriented sample for future rules
- `context.diff`: context-oriented sample for future rules
- `resource.diff`: resource lifecycle sample for future rules
- `db-lifecycle.diff`: database lifecycle sample for future rules
- `realistic-service-risk.diff`: multi-file PR-shaped sample that combines secret, panic, goroutine, context, resource, database, TODO, and missing-test risks
