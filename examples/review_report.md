# Review Report

6 findings, 0 warnings

## Conclusion

- Status: fail
- Reason: blocking_findings
- Summary: Critical or high severity findings require changes before merge.

Metrics: findings=6 total_ms=2144 sandbox_ms=2142 tool_calls=2 permission_blocks=0 redactions=6

Severity Counts:
- critical: 6

Findings: 6

- [CRITICAL] config.go:2 Potential secret appears in added code
  - Evidence: const apiKey=[REDACTED]
  - Recommendation: Replace the literal with a secret manager or environment lookup.
- [CRITICAL] config.go:3 Potential secret appears in added code
  - Evidence: const llmkey=[REDACTED]
  - Recommendation: Replace the literal with a secret manager or environment lookup.
- [CRITICAL] config.go:4 Potential secret appears in added code
  - Evidence: const openaiKey=[REDACTED]
  - Recommendation: Replace the literal with a secret manager or environment lookup.
- [CRITICAL] config.go:5 Potential secret appears in added code
  - Evidence: const client_secret=[REDACTED]
  - Recommendation: Replace the literal with a secret manager or environment lookup.
- [CRITICAL] config.go:6 Potential secret appears in added code
  - Evidence: const bearerToken=[REDACTED]
  - Recommendation: Replace the literal with a secret manager or environment lookup.
- [CRITICAL] config.go:7 Potential secret appears in added code
  - Evidence: const password=[REDACTED]
  - Recommendation: Replace the literal with a secret manager or environment lookup.

## Governance

- Permission allow: scripts/check.sh

## Sandbox

- scripts/check.sh via local-fallback: ok, timeout_ms=30000, output_limit_bytes=65536, duration_ms=2142

## Artifacts

- review_report.json (report): review_report.json
- review_report.md (report): review_report.md
- review_diagnostics.json (diagnostic): review_diagnostics.json
