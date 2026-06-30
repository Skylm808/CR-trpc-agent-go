// Package sandbox 提供轻量受控执行器。
package sandbox

import (
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/governance"
)

// Request 描述待执行命令。
type Request struct {
	Command string
	Args    []string
	Timeout time.Duration
}

// Result 保存执行输出。
type Result struct {
	Stdout string
	Stderr string
}

// Runner 执行已审批命令。
type Runner struct {
	Timeout time.Duration
	Policy  governance.Policy
}

// Run 审批后限时执行命令。
func (r *Runner) Run(ctx context.Context, req Request) (Result, error) {
	decision := r.Policy.Decide(governance.CommandRequest{Command: req.Command, Source: "sandbox"})
	if decision.Action == governance.Deny {
		// 被拒绝命令不进入进程层。
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
	// CommandContext 负责超时中断。
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
