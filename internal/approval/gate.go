// Package approval owns review command approval boundaries.
package approval

import (
	"context"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// AllowedReviewCommands returns the deterministic Go checks this agent may run.
func AllowedReviewCommands(enableStaticcheck bool) []string {
	commands := []string{"go test ./...", "go vet ./..."}
	if enableStaticcheck {
		commands = append(commands, "staticcheck ./...")
	}
	return commands
}

// NewPermissionPolicy builds the fixed command allowlist for CR execution.
func NewPermissionPolicy(skillCommand string, allowedReviewCommands []string) tool.PermissionPolicy {
	return tool.PermissionPolicyFunc(func(ctx context.Context, req *tool.PermissionRequest) (tool.PermissionDecision, error) {
		_ = ctx
		if req == nil {
			return tool.DenyPermission("missing permission request"), nil
		}
		args := string(req.Arguments)
		if req.ToolName == "skill_run" && strings.Contains(args, skillCommand) {
			return tool.AllowPermission(), nil
		}
		if isReviewExecutionTool(req.ToolName) && containsAllowedReviewCommand(args, allowedReviewCommands) {
			return tool.AllowPermission(), nil
		}
		if req.ToolName == "" && containsAllowedReviewCommand(args, allowedReviewCommands) {
			return tool.AllowPermission(), nil
		}
		return tool.AskPermission("unrecognized tool command requires human review"), nil
	})
}

func isReviewExecutionTool(toolName string) bool {
	switch toolName {
	case "workspace_exec", "execute_code":
		return true
	default:
		return false
	}
}

func containsAllowedReviewCommand(args string, commands []string) bool {
	for _, command := range commands {
		if strings.Contains(args, command) {
			return true
		}
	}
	return false
}
