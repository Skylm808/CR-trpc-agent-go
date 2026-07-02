# CI 与评测闭环

本文档说明如何在本地和 CI 中复现 Issue 验收。当前闭环不依赖 LLM API Key，默认只运行确定性规则、公开 fixture 评测和报告/SQLite 回放测试。

## 本地验收入口

```bash
GOCACHE=/private/tmp/cr-agent-gocache scripts/acceptance.sh
```

默认执行：

- `go test ./...`
- `scripts/eval.sh`
- `git diff --check`
- Docker daemon 可用时自动执行 container E2E

Docker 行为由 `CR_AGENT_ACCEPTANCE_DOCKER` 控制：

| 值 | 行为 |
|----|------|
| `auto` 或空 | 默认值；`docker info` 成功时运行 container E2E，否则 skip |
| `always` / `1` / `true` | 强制运行 container E2E；Docker 不可用则失败 |
| `skip` / `never` / `0` / `false` | 跳过 container E2E |

强制验证真实容器沙箱：

```bash
CR_AGENT_ACCEPTANCE_DOCKER=always \
GOCACHE=/private/tmp/cr-agent-gocache \
scripts/acceptance.sh
```

## 宿主 CI 接入

本 example 不提交仓库级 `.github/workflows/*.yml`。GitHub Actions 不是 Issue 硬性要求；未来迁移到官方 `trpc-agent-go/examples` 时，应由宿主仓库决定如何接入 CI。

推荐宿主 CI 拆成两个入口：

- 基础 acceptance：运行 `scripts/acceptance.sh`，显式 `CR_AGENT_ACCEPTANCE_DOCKER=skip`，覆盖公开 fixture、报告字段、SQLite 回放、eval 统计和格式检查。
- container E2E：在有 Docker daemon 的 runner 上设置 `CR_AGENT_ACCEPTANCE_DOCKER=always`，单独验证真实容器沙箱。

GitHub Actions 可按需使用下面的宿主仓库片段：

```yaml
- name: Acceptance
  run: |
    CR_AGENT_ACCEPTANCE_DOCKER=skip \
    GOCACHE=/private/tmp/cr-agent-gocache \
    scripts/acceptance.sh

- name: Container E2E
  if: vars.CR_AGENT_RUN_CONTAINER_E2E == '1'
  run: |
    CR_AGENT_ACCEPTANCE_DOCKER=always \
    GOCACHE=/private/tmp/cr-agent-gocache \
    scripts/acceptance.sh
```

## Hidden Sample 接入

隐藏样本不建议提交到公开仓库。CI 可以通过私有 artifact、临时挂载目录或内部 checkout 提供：

- hidden fixture 根目录
- expected TSV
- 可选报告保存目录

示例：

```bash
CR_AGENT_EVAL_FIXTURES_ROOT=/path/to/hidden-fixtures \
CR_AGENT_EVAL_FIXTURES="hidden-001.diff hidden-002.diff" \
CR_AGENT_EVAL_EXPECTED=/path/to/expected.tsv \
CR_AGENT_EVAL_REPORT_ROOT=/tmp/cr-agent-hidden-reports \
GOCACHE=/private/tmp/cr-agent-gocache \
scripts/eval.sh
```

expected TSV 格式见 [eval-matrix.md](eval-matrix.md)。脚本会输出 `recall`、`precision` 和 `false_positive_rate`；验收口径是：

- `recall >= 0.800`
- `false_positive_rate <= 0.150`
- `missing_findings=0` 和 `unexpected_findings=0` 对公开样本必须满足；隐藏样本可用 optional 行表达允许人工判断的低置信信号。

## 失败回放

设置 `CR_AGENT_EVAL_REPORT_ROOT` 后，`scripts/eval.sh` 会保留每个 fixture 的 `review_report.json`、`review_report.md` 和 `review_diagnostics.json`，用于定位 false positive / false negative。

CI 失败时优先查看：

1. `missing=`：说明 required finding 未命中。
2. `unexpected=`：说明报告出现 expected TSV 未声明的 finding。
3. 单个 fixture 输出目录中的 diagnostics：确认 input metadata、governance、sandbox 和 metrics。
