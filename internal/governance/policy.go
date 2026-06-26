// Package governance 包含第一版命令权限策略。
package governance

import (
	"strings"
)

// Action 是命令对应的标准化治理决策。
type Action string

const (
	// Allow 允许命令继续执行。
	Allow Action = "allow"
	// Deny 阻止命令执行，不再请求人工审批。
	Deny Action = "deny"
	// Ask 表示该命令在执行前需要审批。
	Ask Action = "ask"
	// NeedsHumanReview 为后续交互式流程预留决策状态。
	NeedsHumanReview Action = "needs_human_review"
)

// CommandRequest 描述进入沙箱执行器之前的命令。
type CommandRequest struct {
	Command string
	Source  string
}

// Decision 是下游代码可以持久化或强制执行的策略结果。
type Decision struct {
	Action Action
	Reason string
}

// Policy 是原型使用的确定性权限策略。
type Policy struct{}

// DefaultPolicy 返回保守的第一版策略。
func DefaultPolicy() Policy {
	return Policy{}
}

// Decide 将命令分类为允许、拒绝或需要复核。
func (Policy) Decide(req CommandRequest) Decision {
	cmd := strings.ToLower(strings.TrimSpace(req.Command))
	switch {
	case cmd == "":
		return Decision{Action: Deny, Reason: "empty command"}
	case strings.Contains(cmd, "rm -rf"), strings.Contains(cmd, "sudo "), strings.Contains(cmd, "mkfs"):
		return Decision{Action: Deny, Reason: "high risk command"}
	case strings.HasPrefix(cmd, "go vet"), strings.HasPrefix(cmd, "go test"), strings.HasPrefix(cmd, "staticcheck"):
		return Decision{Action: Allow}
	default:
		return Decision{Action: Ask, Reason: "requires review"}
	}
}
