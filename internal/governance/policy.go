// Package governance contains the first-version command permission policy.
package governance

import (
	"strings"
)

// Action is the normalized governance decision for a command.
type Action string

const (
	// Allow lets the command proceed.
	Allow Action = "allow"
	// Deny blocks the command without asking for human approval.
	Deny  Action = "deny"
	// Ask marks a command as requiring approval before execution.
	Ask   Action = "ask"
	// NeedsHumanReview reserves a decision state for future interactive flows.
	NeedsHumanReview Action = "needs_human_review"
)

// CommandRequest describes a command before it reaches the sandbox runner.
type CommandRequest struct {
	Command string
	Source  string
}

// Decision is the policy result that downstream code can persist or enforce.
type Decision struct {
	Action Action
	Reason string
}

// Policy is the deterministic permission policy used by the prototype.
type Policy struct{}

// DefaultPolicy returns the conservative first-version policy.
func DefaultPolicy() Policy {
	return Policy{}
}

// Decide classifies a command as allowed, denied, or requiring review.
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
