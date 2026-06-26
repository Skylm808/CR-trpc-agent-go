# Framework-first MVP

本文档定义当前小版本边界：在官方 `trpc-agent-go` 上完成 **Skills + Permission + 沙箱 + SQLite 审计** 的最小闭环，并继续补齐 Issue #2004 验收项。

## 已完成的纠偏

项目已从纯本地 diff runner 调整为框架优先路线：

- `go.mod` 依赖官方 `trpc.group/trpc-go/trpc-agent-go v1.10.0`，没有本地 `replace`。
- `internal/agent` 统一编排 CLI，不再让 CLI 直接调用本地 runner。
- `tool/skill` 加载并运行 `skills/code-review/scripts/check.sh`。
- `tool.PermissionPolicy` 在执行前做 allow/ask/deny 决策。
- `codeexecutor/container` 是默认 runtime；`local-fallback` 仅显式用于测试/开发。
- `tool/codeexec` 在 `sandbox` 模式执行 `go test ./...`、`go vet ./...`、可选 `staticcheck ./...`。
- SQLite 保存 task、permission/filter decision、sandbox run、finding、artifact、metrics、report。

## 当前小版本能力

```text
CLI 输入
  -> internal/agent.Agent
  -> skill_load(code-review)
  -> PermissionPolicy
  -> skill_run(scripts/check.sh)
  -> optional tool/codeexec(go test/go vet/staticcheck)
  -> redact + dedupe + warning/human-review split
  -> SQLite audit trail
  -> review_report.json / review_report.md
```

## 当前可运行路径

开发/测试环境没有 Docker 时：

```bash
go run ./cmd/review-agent \
  --fixture secret.diff \
  --runtime local-fallback \
  --mode rule-only
```

接近生产路径：

```bash
go run ./cmd/review-agent \
  --repo-path /path/to/go/repo \
  --runtime container \
  --mode sandbox \
  --staticcheck
```

## 能力状态

| 能力 | 当前状态 | 下一步 |
|------|----------|--------|
| Skill | ✅ `tool/skill` load/run 已接入 | 增加脚本输出 schema 校验 |
| 沙箱 | 🔶 container 默认，local fallback 可测；Docker E2E env-gated test 已加 | 在有 Docker daemon 的 CI/机器上执行 |
| Permission | ✅ allowlist 与 ask/deny Agent E2E 已接入 | 扩展更细粒度命令策略 |
| 输入 | 🔶 diff/fixture/repo 支持 | 补文件路径列表、base/head ref |
| 规则 | ✅ 覆盖 8 类公开 fixture | 增加 hidden/eval 评测 |
| 存储 | ✅ SQLite 核心表和查询方法完成 | 考虑迁出 `internal/storage/store.go` 独立接口 |
| 报告 | ✅ 核心摘要字段完成 | 增加 conclusion 字段和更稳定 golden report |
| 安全 | 🔶 timeout/output limit/digest/redaction 有记录 | 增加 artifact cap、env 白名单强校验 |
| 监控 | 🔶 metrics 表记录核心摘要 | 接官方 telemetry hook |

## 非目标

- 不 fork 或复制 `trpc-agent-go` 框架代码。
- 不把 `local-fallback` 当生产默认方案。
- 不用纯文本 LLM 评论替代 Skill、Permission、沙箱和数据库链路。
- 不优先做复杂 AST；当前更重要的是验收链路和审计数据完整。

## 下一阶段 v0.2 Definition of Done

- `go test ./...` 通过。
- 所有公开 fixture 在 `local-fallback` 下通过 rule_id/severity/status 断言。
- container integration test 已新增，默认跳过，设置环境变量后跑真实 Docker。
- `sandbox` 模式的 go test/go vet/staticcheck run 均有 permission decision 和 sandbox run。
- ask/deny 非 allow 决策不会进入 executor，并会进入报告治理摘要和 SQLite。
- SQLite 可按 task_id 查询 task、permission/filter decision、sandbox run、finding、artifact、metrics、report。
- 报告和数据库中无明文 API key/token/password。
- README、docs、examples 与 CLI flag 和 JSON contract 保持一致。

## 相关文档

- [architecture.md](architecture.md)
- [implementation-plan.md](implementation-plan.md)
- [issue-2004-traceability.md](issue-2004-traceability.md)
- [goal-prompt-framework-mvp.md](goal-prompt-framework-mvp.md)
