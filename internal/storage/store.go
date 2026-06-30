// Package storage 定义持久化边界。
package storage

import (
	"context"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
	"github.com/Skylm808/CR-trpc-agent-go/internal/storage/sqlite"
)

// Task 是审查任务。
type Task = sqlite.Task

// DecisionRecord 是权限决策记录。
type DecisionRecord = sqlite.DecisionRecord

// FilterDecisionRecord 是过滤决策记录。
type FilterDecisionRecord = sqlite.FilterDecisionRecord

// SandboxRunRecord 是沙箱运行记录。
type SandboxRunRecord = sqlite.SandboxRunRecord

// ArtifactRecord 是产物记录。
type ArtifactRecord = sqlite.ArtifactRecord

// MetricsRecord 是指标记录。
type MetricsRecord = sqlite.MetricsRecord

// Store 定义最小存储能力。
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
