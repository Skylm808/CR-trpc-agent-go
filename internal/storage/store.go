// Package storage 定义审查 Agent 使用的持久化边界。
package storage

import (
	"context"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
	"github.com/Skylm808/CR-trpc-agent-go/internal/storage/sqlite"
)

// Task 是一次审查任务的持久化锚点。
type Task = sqlite.Task

// DecisionRecord 是一次 PermissionPolicy 决策的审计记录。
type DecisionRecord = sqlite.DecisionRecord

// FilterDecisionRecord 是一次过滤或脱敏决策的审计记录。
type FilterDecisionRecord = sqlite.FilterDecisionRecord

// SandboxRunRecord 是一次沙箱执行尝试的审计记录。
type SandboxRunRecord = sqlite.SandboxRunRecord

// ArtifactRecord 是一次报告或沙箱产物的持久化引用。
type ArtifactRecord = sqlite.ArtifactRecord

// MetricsRecord 是一次审查的聚合监控记录。
type MetricsRecord = sqlite.MetricsRecord

// Store 定义 Agent 需要的最小持久化能力，便于后续替换 SQL 后端。
type Store interface {
	SaveTask(context.Context, Task) error
	SaveFinding(context.Context, string, review.Finding) error
	SaveDecision(context.Context, DecisionRecord) error
	SaveFilterDecision(context.Context, FilterDecisionRecord) error
	SaveSandboxRun(context.Context, SandboxRunRecord) error
	SaveArtifact(context.Context, ArtifactRecord) error
	SaveMetrics(context.Context, MetricsRecord) error
	SaveReport(context.Context, string, []byte, []byte) error
	Close() error
}
