# Issue 验收证据

本文档面向最终验收：把 Issue 要求映射到可运行命令、代码路径、测试和 sample 输出。当前实现是基于 `trpc-agent-go` Tool / Skill / CodeExecutor / PermissionPolicy / artifact / telemetry / SQLite 的 CLI Agent 原型；尚未接入 Runner/Event、Session/Memory 和 E2B runtime。

## 验收命令

```bash
GOCACHE=/private/tmp/cr-agent-gocache scripts/acceptance.sh
```

该脚本默认执行以下基础门禁：

```bash
GOCACHE=/private/tmp/cr-agent-gocache go test ./...
GOCACHE=/private/tmp/cr-agent-gocache scripts/eval.sh
git diff --check
```

Docker 可用时 `scripts/acceptance.sh` 会自动追加 container E2E；也可以强制运行：

```bash
CR_AGENT_ACCEPTANCE_DOCKER=always \
GOCACHE=/private/tmp/cr-agent-gocache \
scripts/acceptance.sh
```

CI 集成由宿主仓库负责；本 example 保留 repo-neutral 脚本，接入方式见 [ci.md](ci.md)。

## 框架模块映射

| Issue 能力 | 当前实现 | 证据 |
|------------|----------|------|
| Skill 加载与执行 | `tool/skill` 的 `skill_load` / `skill_run` 执行 `skills/code-review/scripts/check.sh` | `internal/agent/execution.go`、`skills/code-review/SKILL.md` |
| workspace 级脚本 | `tool/workspaceexec` 执行 `go test ./...`、`go vet ./...`、可选 `staticcheck ./...` | `internal/agent/execution.go`、`TestAgentRunContainerRuntimeExecutesGoChecks` |
| CodeExecutor 沙箱 | 默认 `codeexecutor/container`，`local-fallback` 仅开发测试；`tool/codeexec` 是 Go checks fallback | `internal/agent/execution.go`、README runtime 说明 |
| Permission 治理 | 所有 `skill_run` / Go check 命令先过 `tool.PermissionPolicy`，非 allow 不执行 | `TestAgentRunDoesNotExecuteNonAllowPermission` |
| artifact | `review_report.json`、`review_report.md`、`review_diagnostics.json` 写入官方 artifact service，本地文件和 SQLite 引用继续保留 | `TestArtifactServiceReportsCanBeSavedAsArtifacts`、`TestAgentDefaultArtifactService` |
| telemetry | 官方 trace span 记录 task、mode、runtime、输入类型、耗时、tool 调用数、permission block、finding/artifact 数、severity/exception 分布和结论；SQLite metrics 表保存聚合指标 | `TestAgentRunRecordsTelemetryAttributes`、`TestAcceptanceEvidenceReportsAndSQLiteReplay` |
| SQLite 审计 | task、finding、permission/filter decision、sandbox run、artifact、metrics、report 按 task id 查询 | `TestAcceptanceEvidenceReportsAndSQLiteReplay` |

沙箱安全边界的集中证据见 [sandbox-safety.md](sandbox-safety.md)，包括非 allow 不执行、失败/超时不崩溃、输出限制、env whitelist、digest、脱敏和 artifact cap。

## 输入输出验收

| 要求 | 运行方式 | 证据 |
|------|----------|------|
| `--fixture` | `go run ./cmd/review-agent --fixture secret-shapes.diff --fixtures-root testdata/fixtures --runtime local-fallback --mode rule-only --output-dir examples` | `examples/review_report.json`、`examples/review_report.md`、`examples/review_diagnostics.json` |
| `--diff-file` | `go run ./cmd/review-agent --diff-file testdata/fixtures/panic.diff --runtime local-fallback --mode fake-model --output-dir /tmp/review-out` | `TestRunCanUseFixtureName`、README 示例 |
| `--repo-path` | `go run ./cmd/review-agent --repo-path /path/to/go/repo --runtime container --mode sandbox --sqlite /tmp/review.db --output-dir /tmp/review-out` | `TestReadInputFromRepoReadsWorkingTreeDiff`、container E2E |
| 报告输出 | JSON / Markdown / diagnostics 三个文件 | `examples/`、`TestAcceptanceEvidenceReportsAndSQLiteReplay` |
| SQLite 回放 | 通过 task id 查询全链路审计数据 | `TestAcceptanceEvidenceReportsAndSQLiteReplay` |

## 公开样本

`testdata/fixtures/` 当前有 14 条公开样本，覆盖无问题、安全、敏感信息脱敏、panic、TODO、测试缺失、goroutine/context/resource/db 生命周期、重复 finding、沙箱失败和超时。`cmd/review-agent/fixtures_test.go` 会逐条运行并校验 rule_id、severity、status 和脱敏结果；`scripts/eval.sh` 输出公开样本 recall、precision 和 false positive rate，并对内置公开矩阵要求 `missing_findings=0`、`unexpected_findings=0`。

隐藏样本通过 `CR_AGENT_EVAL_FIXTURES_ROOT`、`CR_AGENT_EVAL_FIXTURES`、`CR_AGENT_EVAL_EXPECTED` 和 `CR_AGENT_EVAL_REPORT_ROOT` 注入；`cmd/review-agent/eval_script_test.go` 固定验证外部 expected TSV、optional 行、阈值失败和报告保留路径。验收阈值默认按 Issue 要求执行：`CR_AGENT_EVAL_MIN_RECALL=0.800` 且 `CR_AGENT_EVAL_MAX_FALSE_POSITIVE_RATE=0.150`，不达标时脚本非 0 退出。

## 报告字段验收

报告必须包含 findings 摘要、severity 统计、人工复核项、治理拦截摘要、监控指标、沙箱执行摘要和修复建议。当前由 `TestReportsIncludeGovernanceSandboxArtifactsAndHumanReviewContract` 和 `TestAcceptanceEvidenceReportsAndSQLiteReplay` 共同验证字段存在，sample 输出见：

- `examples/review_report.json`
- `examples/review_report.md`
- `examples/review_diagnostics.json`

## SQLite 回放证据

`TestAcceptanceEvidenceReportsAndSQLiteReplay` 运行 `secret-shapes.diff` 后读取报告中的 `task_id`，并查询：

- `review_tasks`
- `findings`
- `permission_decisions`
- `filter_decisions`
- `sandbox_runs`
- `artifacts`
- `metrics`
- `reports`

README 中保留等价 SQL 查询示例。`examples/review.db` 可本地生成，但因 `*.db` 被忽略，不作为文本交付物提交。

## 尚未接入模块

- Runner/Event：当前是单次 CLI 审查，不需要流式输出、多轮恢复或 Web/UI 实时观察；后续接 UI 或长任务恢复时再引入。
- Session/Memory：当前 SQLite 审计已满足单次任务持久化；跨 PR、多轮经验复用时再映射到 `session/sqlite` 和 memory。
- E2B：当前 runtime 默认 container，E2B 适合作为远端沙箱扩展；不影响现阶段 container 验收。

后续迁移到官方 `trpc-agent-go/examples` 的路径和目录边界见 [upstream-example-migration.md](upstream-example-migration.md)。

## 当前剩余风险

- 隐藏样本的 80% 检出率和 15% 误报率已具备脚本门禁，真实达标仍需要外部 hidden fixture + expected matrix 持续校准。
- Docker container E2E 依赖本机或宿主 CI Docker daemon；建议用 `CR_AGENT_ACCEPTANCE_DOCKER=always` 单独启用。
- 未接 LLM 时只能发现确定性规则覆盖的风险，复杂业务逻辑错误仍需后续模型审查或更多规则补齐。
