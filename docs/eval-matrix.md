# Eval Matrix

本文档说明 `scripts/eval.sh` 的公开样本评测方式、`scripts/holdout_eval.sh` 的自包含 holdout/adversarial 验收，以及接入外部 hidden sample 时的输入契约。

## 当前公开评测

默认评测脚本会读取 `testdata/fixtures/`，逐个运行 `cmd/review-agent`，并把每个 fixture 的 JSON 报告与预期矩阵做对比。

```bash
GOCACHE=/private/tmp/cr-agent-gocache scripts/eval.sh
```

本地和 CI 的统一入口是：

```bash
GOCACHE=/private/tmp/cr-agent-gocache scripts/acceptance.sh
```

## Holdout / Adversarial 验收

仓库提交了 `testdata/holdout/` 作为 self-contained holdout matrix。它不是私有 hidden 数据，而是 public fixture 之外的本地验收集，用来覆盖 false-positive guardrail、组合生命周期风险和无 API key 的 fake-model 语义增量路径。

```bash
GOCACHE=/private/tmp/cr-agent-gocache scripts/holdout_eval.sh
```

该脚本固定使用 `matrix_source=holdout`，默认以 `fake-model` 模式运行，因此 `model-semantic.diff` 可以证明模型合并链路产生增量 finding，而不依赖 DeepSeek/OpenAI API key。

可选环境变量：

- `CR_AGENT_EVAL_FIXTURES_ROOT`：外部 fixture 根目录。
- `CR_AGENT_EVAL_SKILLS_ROOT`：Skill 根目录。
- `CR_AGENT_EVAL_RUNTIME`：`container` 或 `local-fallback`。
- `CR_AGENT_EVAL_MODE`：默认 `rule-only`。
- `CR_AGENT_EVAL_FIXTURES`：样本子集，空格分隔。
- `CR_AGENT_EVAL_MATRIX`：外部 expected matrix TSV 路径；缺失文件会清晰失败。
- `CR_AGENT_EVAL_EXPECTED`：旧兼容别名，仅在未设置 `CR_AGENT_EVAL_MATRIX` 时生效。
- `CR_AGENT_EVAL_REPORT_ROOT`：保留每个 fixture 的输出报告目录；默认使用临时目录。
- `CR_AGENT_EVAL_MIN_RECALL`：召回率下限，默认 `0.800`。
- `CR_AGENT_EVAL_MAX_FALSE_POSITIVE_RATE`：误报率上限，默认 `0.150`。

输出字段包括：

- `fixtures`
- `expected`
- `required_expected`
- `optional_expected`
- `true_positive`
- `false_positive`
- `false_negative`
- `recall`
- `precision`
- `false_positive_rate`
- `missing_findings`
- `unexpected_findings`
- `duration_ms`
- `matrix_source`

## External Hidden Matrix 格式

隐藏样本使用 TSV，每行四列或五列。四列旧格式默认 `required=true`：

```text
fixture_name	rule_id	severity	status
```

五列格式可以显式声明该项是否必须检出：

```text
fixture_name	rule_id	severity	status	required
```

`required` 可取 `true` / `required` / `yes` / `1` / `must`，或 `false` / `optional` / `no` / `0`。optional 项命中时不算误报，未命中也不算漏报，适合记录低置信或允许人工判断的信号。

示例：

```text
hidden-001.diff	secret-leak	critical	finding	true
hidden-002.diff	missing-test-hint	low	warning	false
```

运行 hidden matrix：

```bash
CR_AGENT_EVAL_FIXTURES_ROOT=/path/to/hidden-fixtures \
CR_AGENT_EVAL_FIXTURES="hidden-001.diff hidden-002.diff" \
CR_AGENT_EVAL_MATRIX=/path/to/expected.tsv \
GOCACHE=/private/tmp/cr-agent-gocache \
scripts/eval.sh
```

这样可以把公开评测和隐藏评测统一到同一条执行路径上，减少 CI 差异。

## 评分口径

- `true_positive`：required 项中存在且报告中命中的 `(rule_id, severity, status)`。
- `false_positive`：报告中出现但 matrix 未声明的项。
- `false_negative`：required 项声明但报告未出现的项。
- `recall`：`TP / (TP + FN)`。
- `precision`：`TP / (TP + FP)`。
- `false_positive_rate`：`FP / (TP + FP)`。
- `missing_findings`：漏检项数量；非零时脚本会输出 `missing=` 明细。
- `unexpected_findings`：未声明项数量；非零时脚本会输出 `unexpected=` 明细。

Issue 验收阈值：

- 高危检出率：`recall >= 0.800`。
- 误报率：`false_positive_rate <= 0.150`。
- 公开样本：必须 `false_positive=0`、`false_negative=0`。
- 隐藏样本：允许用第五列 optional 标记低置信或可人工判断项；optional 命中不算误报，未命中不算漏报。

脚本会先检查阈值，再对内置公开矩阵执行 exact match。阈值失败会非 0 退出，并输出：

```text
threshold_failed=recall actual=0.750 min=0.800
threshold_failed=false_positive_rate actual=0.250 max=0.150
```

公开样本使用内置矩阵时，即使 recall / false positive rate 达标，只要存在漏检或未声明 finding，也会失败：

```text
threshold_failed=public_matrix_exact_match missing_findings=1 unexpected_findings=0
```

## CI 注入示例

```bash
CR_AGENT_EVAL_FIXTURES_ROOT="$RUNNER_TEMP/hidden-fixtures" \
CR_AGENT_EVAL_FIXTURES="hidden-001.diff hidden-002.diff hidden-003.diff" \
CR_AGENT_EVAL_MATRIX="$RUNNER_TEMP/expected.tsv" \
CR_AGENT_EVAL_REPORT_ROOT="$RUNNER_TEMP/cr-agent-reports" \
CR_AGENT_EVAL_MIN_RECALL=0.800 \
CR_AGENT_EVAL_MAX_FALSE_POSITIVE_RATE=0.150 \
GOCACHE="$RUNNER_TEMP/cr-agent-gocache" \
scripts/eval.sh
```

如果 Docker daemon 不可用，基础 CI 使用：

```bash
CR_AGENT_ACCEPTANCE_DOCKER=skip scripts/acceptance.sh
```

如果 Docker daemon 可用并且希望把 container sandbox 作为必过门禁：

```bash
CR_AGENT_ACCEPTANCE_DOCKER=always scripts/acceptance.sh
```

## 建议

`testdata/holdout/` 已经提供本仓库自包含的非公开矩阵替代物。真正私有 hidden sample 不建议提交到公开仓库；如 reviewer/CI 另有私有样本，可以通过 artifact 或 secret volume 提供 fixture root 和 expected matrix，并设置 `CR_AGENT_EVAL_REPORT_ROOT` 保留每个样本的 `review_report.json`、`review_report.md` 和 `review_diagnostics.json` 用于回放。
