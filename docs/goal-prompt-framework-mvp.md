# Goal Prompt：基于 trpc-agent-go 的 CR Agent MVP

下面这段可以直接复制给 Goal 模式。

```text
你在仓库 /Users/skylm/Desktop/GOLAND/trpc-agent/CR-trpc-agent-go 中继续开发 Issue #2004：基于官方 trpc-agent-go 的 Skills + 沙箱 + 数据库存储自动代码评审 Agent。不要从零重写，不要做纯本地 diff 扫描器，不要 fork trpc-agent-go。当前仓库已经接入官方 trpc-agent-go v1.10.0、tool/skill、tool/codeexec、tool.PermissionPolicy、codeexecutor/container、local fallback 和 SQLite 审计链路。你的目标是在现有基础上完成第一个可验收 MVP。

目标：
1. 保持官方 trpc-agent-go 为主线：tool/skill 加载并运行 skills/code-review，tool.PermissionPolicy 决策所有高风险命令，codeexecutor/container 是默认 runtime，local-fallback 只能显式用于开发和测试。
2. 补齐真实沙箱和治理验证：增加 container runtime integration test（默认跳过，设置环境变量才跑 Docker），补 Agent 层 ask/deny/needs_human_review 不进入 executor 的测试，确保 go test/go vet/staticcheck 都有 permission decision 和 sandbox run。
3. 补齐数据库和安全验收：按 task_id 可查询 task、permission/filter decision、sandbox run、finding、artifact、metrics、report；增加 DB 全表 secret 扫描测试，确保报告和数据库不出现明文 API key/token/password。
4. 补齐评测和交付：新增 hidden/eval 或公开 fixture eval 脚本，输出 recall、precision、耗时；保持 examples/review_report.json 和 examples/review_report.md 可生成；更新 README 和 docs。
5. 保持 deterministic rule-only / dry-run / fake-model 在无真实模型 API Key 时可完整跑通，单次流程耗时 <= 2 分钟。

约束：
- 不能使用本地 replace 指向 trpc-agent-go 源码；只能依赖官方 module。
- 不能把 local runtime 当生产默认；默认必须是 container。
- 高风险命令的 deny / ask / needs_human_review 不能进入 executor。
- 沙箱失败、超时、命令失败不能让整个 review 任务崩溃，必须生成报告并落库失败记录。
- 所有新增或修改的 Go 方法，方法外和关键方法内都加清晰中文注释。
- 每个模块小步提交，commit 使用英文 conventional commits，并带 Lore trailer（Constraint/Rejected/Confidence/Scope-risk/Tested/Not-tested）。
- 不要删除 docs；docs 是交付物。只忽略根目录生成的 review_report.json/md，examples 下的示例报告需要提交。

优先执行顺序：
1. 先运行 git status、阅读 docs/architecture.md、docs/implementation-plan.md、docs/data-contract.md、docs/issue-2004-traceability.md、README.md，确认当前状态。
2. 写或补测试：container integration test（env gated）、permission non-allow E2E、DB secret scan、eval script 测试。
3. 再实现最小代码改动：优先修边界和审计，不做无关重构。
4. 更新 docs 和 examples。
5. 运行 GOCACHE=/private/tmp/cr-agent-gocache go test ./...，再运行一次 CLI fixture 样例生成报告。
6. 检查 git diff --check、git status，确认没有明文 secret 输出。
7. 分模块提交，最后给出剩余风险。

验收证据：
- go test ./... 通过。
- 公开 fixtures 全部通过 rule_id/severity/status 断言。
- SQLite 中能按 task_id 查询 task、sandbox run、permission decision、filter decision、finding、artifact、metrics、report。
- review_report.json/md 包含 findings 摘要、severity 统计、human_review_items、governance_summary、sandbox_summary、metrics、artifacts、修复建议。
- sandbox fail/timeout 不崩溃。
- 报告和数据库中无明文 API key/token/password。
- container runtime 有可运行的 integration test 或明确说明因本地 Docker 不可用而跳过。
```

## 使用建议

如果你只想让 Goal 模式做一个最小安全推进，可以把目标缩短为：

```text
继续 CR-trpc-agent-go 的 trpc-agent-go framework-first MVP，只做三件事：补 container integration test（env gated）、补 permission non-allow Agent E2E、补 DB 全表 secret 扫描测试。不得重写架构，不得绕过 tool/skill、PermissionPolicy 或 codeexecutor/container。完成后运行 go test ./...，更新 docs，并按 conventional commit + Lore trailers 提交。
```
