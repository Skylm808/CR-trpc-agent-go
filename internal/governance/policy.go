// Package governance 提供命令权限策略。
package governance

import (
	"strings"
)

// Action 是治理决策。
type Action string

const (
	// Allow 允许命令继续执行。
	Allow Action = "allow"
	// Deny 阻止命令执行。
	Deny Action = "deny"
	// Ask 表示该命令在执行前需要审批。
	Ask Action = "ask"
	// NeedsHumanReview 表示需要人工复核。
	NeedsHumanReview Action = "needs_human_review"
)

// CommandRequest 描述待审批命令。
type CommandRequest struct {
	Command string
	Source  string
}

// Decision 是策略结果。
type Decision struct {
	Action Action
	Reason string
}

// Policy 是确定性权限策略。
type Policy struct{}

// DefaultPolicy 返回默认策略。
func DefaultPolicy() Policy {
	return Policy{}
}

// Decide 判断命令是否可执行。
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
