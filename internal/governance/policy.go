package governance

import (
	"strings"
)

type Action string

const (
	Allow Action = "allow"
	Deny  Action = "deny"
	Ask   Action = "ask"
	NeedsHumanReview Action = "needs_human_review"
)

type CommandRequest struct {
	Command string
	Source  string
}

type Decision struct {
	Action Action
	Reason string
}

type Policy struct{}

func DefaultPolicy() Policy {
	return Policy{}
}

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

