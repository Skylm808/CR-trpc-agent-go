package agent

import (
	"github.com/Skylm808/CR-trpc-agent-go/internal/approval"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// defaultPermissionPolicy 返回代码审查命令的固定 allowlist。
func defaultPermissionPolicy() tool.PermissionPolicy {
	return approval.NewPermissionPolicy(defaultSkillCommand, approval.AllowedReviewCommands(true))
}
