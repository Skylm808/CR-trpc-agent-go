# 沙箱安全边界矩阵

本文档集中说明当前 CR Agent 的沙箱安全边界、模型输入输出过滤边界、审计字段和测试证据。当前实现是基于 `trpc-agent-go` Tool / Skill / CodeExecutor / PermissionPolicy / workspaceexec 的 CLI Agent 原型，不是生产多租户隔离平台；生产部署仍应由宿主环境提供容器运行策略、网络策略和密钥注入策略。

## 执行边界

| 边界 | 当前策略 | 证据 |
|------|----------|------|
| 默认 runtime | `container`，基于官方 `codeexecutor/container` | `internal/agent.newExecutor` |
| 本地 fallback | 只能显式选择 `local-fallback`，用于开发和测试 | `TestLocalFallbackExecutorsUseIsolatedWorkDirs`、README |
| Skill 执行 | 只允许 `skills/code-review/scripts/check.sh` | `toolskill.WithAllowedCommands`、`defaultPermissionPolicy` |
| Go 检查 | 只允许 `go test ./...`、`go vet ./...`、显式 `staticcheck ./...` | `defaultPermissionPolicy`、sandbox mode tests |
| 非 allow 决策 | `deny` / `ask` / `needs_human_review` 不进入 executor | `TestAgentRunDoesNotExecuteNonAllowPermission` |
| workspace 执行 | 优先用官方 `tool/workspaceexec`，失败时才用 `tool/codeexec` fallback | `TestRunGoSandboxCommandPrefersWorkspaceExec`、`TestRunGoSandboxCommandFallsBackToCodeExec` |
| 模型审查 | `fake-model` 默认只调用本地 deterministic provider；显式 `--model-provider http|openai|deepseek` 才调用外部 provider。OpenAI-compatible 兼容 `OPENAI_API_KEY` / `OPENAI_BASE_URL`，DeepSeek 默认 `DEEPSEEK_API_KEY`。prompt 输入先脱敏，provider output 再脱敏 | `TestModelProviderRedactsInputOutputReportsAndSQLite`、`TestHTTPModelProviderCallsServerAndMergesFindings`、`TestOpenAIModelProviderBuildsOfficialDeepSeekModel` |

## 审计字段

每次 Skill、workspaceexec 或 codeexec fallback 都会生成 `sandbox_runs` 记录，并进入 `review_report.json` / `review_diagnostics.json` 的 `sandbox_summary`。

| 字段 | 作用 | 测试证据 |
|------|------|----------|
| `command` | 保留审计命令，不暴露容器内实现细节 | sandbox / permission tests |
| `runtime` | 标记 `container`、`local-fallback` 或 `e2b` | container E2E、E2B unsupported test |
| `status` | `ok` / `failed` / `error` / `timed_out` / `skipped` / `unsupported` / permission action | failure / timeout / dry-run / E2B tests |
| `timeout_ms` | 固定每次执行超时边界 | failure / timeout tests |
| `output_limit_bytes` | 固定输出上限 | failure / timeout tests |
| `env_whitelist` | 记录允许进入执行环境的环境变量名 | dry-run / sandbox tests |
| `stdout_digest` / `stderr_digest` | 用摘要保留失败线索，避免保存完整输出 | failure tests |
| `output` | 兼容字段，只保存脱敏且受限的失败线索 | Go check failure test |

模型审计不写入 `sandbox_runs`，而是进入 `metrics`、`review_diagnostics.json` 和 telemetry trace：`model_call_count`、`model_duration_ms`、`model_finding_count`、`model_exception_count`。模型 finding 本身复用 `findings` 表，通过 `source=model` 或 `source=fake_model` 区分。

## 失败处理

沙箱失败和超时不会让整个 review 崩溃。Agent 会：

1. 保存 `sandbox_runs.status` 为 `failed` 或 `timed_out`。
2. 增加 `metrics.exception_counts["sandbox_failed"]`。
3. 在 conclusion 中进入 `needs_human_review`。
4. 继续生成 `review_report.json`、`review_report.md` 和 `review_diagnostics.json`。

对应测试：

- `TestAgentRunRecordsSandboxFailureWithoutCrashing`
- `TestAgentRunRecordsSandboxTimeoutWithoutCrashing`
- `TestAgentRunSandboxModeRecordsGoCheckFailure`

## 内容安全

所有 finding evidence、报告正文、SQLite 文本列和失败输出写入前都必须脱敏或摘要化。

| 风险 | 当前策略 | 证据 |
|------|----------|------|
| API key / token / password 明文 | Skill 脚本和 Agent 双层脱敏 | `TestAgentRunRedactsCommonSecretShapesInReportsAndSQLite` |
| Skill 输出重复或未脱敏 | Agent 层 `sanitizeFinding` 兜底 | `TestParseSkillFindingsDedupesAndRedacts` |
| SQLite 泄漏 | 全表文本列扫描 raw secret | secret redaction tests |
| artifact 过大 | 写本地和 artifact service 前先检查大小 | `TestAgentRunRejectsOversizedArtifacts` |
| model prompt/output 泄漏 | prompt diff summary 和 provider output evidence 均经 Agent 脱敏；外部 provider API key 只来自 env，本地 `cr-agent.yaml` 已被 gitignore 忽略，报告/diagnostics/SQLite 不保存 key 或 key env 名 | `TestModelProviderRedactsInputOutputReportsAndSQLite`、`TestHTTPModelProviderCallsServerAndMergesFindings`、`TestRunDeepSeekProviderMissingAPIKeyDoesNotAbort` |

## 未完成边界

- E2B / Cube 真实 runtime 尚未接入；当前 `--runtime e2b` 只生成显式 unsupported 审计记录，不静默 fallback。
- Claude / Gemini 厂商 SDK provider 尚未接入；当前 fake provider、opt-in HTTP provider 和官方 `model/openai` OpenAI-compatible / DeepSeek provider 验证边界、脱敏、分流、审计和失败降级。
- 官方 metric exporter / OTLP dashboard 尚未部署；当前使用官方 telemetry trace span 和 SQLite metrics。
- 当前 env whitelist 是审计边界，容器级强环境隔离仍依赖部署侧 executor 配置。
- 复杂业务逻辑错误不靠 deterministic 规则保证，真实检出率取决于外部模型端点或后续领域规则补齐。
