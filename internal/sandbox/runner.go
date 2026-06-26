// Package sandbox 提供第一版审查 Agent 使用的轻量受控命令执行器。
package sandbox

import (
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/governance"
)

// Request 描述一条在沙箱约束下执行的命令。
type Request struct {
	Command string
	Args    []string
	Timeout time.Duration
}

// Result 保存沙箱命令的标准输出和标准错误。
type Result struct {
	Stdout string
	Stderr string
}

// Runner 在策略允许后，以受限超时执行命令。
type Runner struct {
	Timeout time.Duration
	Policy  governance.Policy
}

// Run 先做策略检查，再强制超时，并在失败时把错误返回给调用方而不是崩溃。
func (r *Runner) Run(ctx context.Context, req Request) (Result, error) {
	decision := r.Policy.Decide(governance.CommandRequest{Command: req.Command, Source: "sandbox"})
	if decision.Action == governance.Deny {
		// 被拒绝的命令不会进入宿主进程层。
		return Result{}, errors.New(decision.Reason)
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = r.Timeout
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// CommandContext 即使命令挂起，也会把沙箱限制在超时时间内。
	cmd := exec.CommandContext(ctx, req.Command, req.Args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return Result{}, ctx.Err()
	}
	if err != nil {
		return Result{Stdout: string(out)}, err
	}
	return Result{Stdout: string(out)}, nil
}
