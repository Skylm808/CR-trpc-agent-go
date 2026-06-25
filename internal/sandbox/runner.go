package sandbox

import (
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/governance"
)

type Request struct {
	Command string
	Args    []string
	Timeout time.Duration
}

type Result struct {
	Stdout string
	Stderr string
}

type Runner struct {
	Timeout time.Duration
	Policy  governance.Policy
}

func (r *Runner) Run(ctx context.Context, req Request) (Result, error) {
	decision := r.Policy.Decide(governance.CommandRequest{Command: req.Command, Source: "sandbox"})
	if decision.Action == governance.Deny {
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

