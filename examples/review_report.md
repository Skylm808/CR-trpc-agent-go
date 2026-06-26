# Review Report

1 findings, 0 warnings

Metrics: findings=1 total_ms=2212 sandbox_ms=2211 tool_calls=2 permission_blocks=0 redactions=1

Severity Counts:
- critical: 1

Findings: 1

- [CRITICAL] config.go:1 Potential secret appears in added code
  - Evidence: const apiKey=[REDACTED]
  - Recommendation: Replace the literal with a secret manager or environment lookup.

## Governance

- Permission allow: scripts/check.sh

## Sandbox

- scripts/check.sh via local-fallback: ok, timeout_ms=30000, output_limit_bytes=65536, duration_ms=2211

## Artifacts

- review_report.json (report): review_report.json
- review_report.md (report): review_report.md
