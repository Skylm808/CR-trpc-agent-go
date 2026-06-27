# Eval Matrix

本文档说明 `scripts/eval.sh` 的公开样本评测方式，以及后续接入 hidden sample 时推荐的输入契约。

## 当前公开评测

默认评测脚本会读取 `testdata/fixtures/`，逐个运行 `cmd/review-agent`，并把每个 fixture 的 JSON 报告与预期矩阵做对比。

```bash
GOCACHE=/private/tmp/cr-agent-gocache scripts/eval.sh
```

可选环境变量：

- `CR_AGENT_EVAL_FIXTURES_ROOT`：外部 fixture 根目录。
- `CR_AGENT_EVAL_SKILLS_ROOT`：Skill 根目录。
- `CR_AGENT_EVAL_RUNTIME`：`container` 或 `local-fallback`。
- `CR_AGENT_EVAL_MODE`：默认 `rule-only`。
- `CR_AGENT_EVAL_FIXTURES`：样本子集，空格分隔。

## 推荐的 hidden matrix 格式

隐藏样本建议用与公开样本相同的 TSV 结构，每行四列：

```text
fixture_name	rule_id	severity	status
```

示例：

```text
hidden-001.diff	secret-leak	critical	finding
hidden-002.diff	missing-test-hint	low	warning
```

脚本在读取 hidden matrix 时，只需要额外接收一个预期文件路径，不需要修改审查逻辑本身。这样可以把公开评测和隐藏评测统一到同一条执行路径上，减少 CI 差异。

## 评分口径

- `true_positive`：fixture 中存在且报告中命中的 `(rule_id, severity, status)`。
- `false_positive`：报告中出现但 matrix 未声明的项。
- `false_negative`：matrix 声明但报告未出现的项。
- `recall`：`TP / (TP + FN)`。
- `precision`：`TP / (TP + FP)`。

## 建议

后续如果要把 hidden sample 接入 CI，建议让脚本支持：

1. `CR_AGENT_EVAL_EXPECTED=/path/to/expected.tsv`
2. `CR_AGENT_EVAL_REPORT_ROOT=/path/to/output`
3. `CR_AGENT_EVAL_JSON_ONLY=1`

这样可以把数据集和执行器解耦，便于复用。
