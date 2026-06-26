// Package sandbox provides a tiny guarded command runner used by the first
// version of the review agent.
package sandbox

import (
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/governance"
)

// Request describes one command to execute under the sandbox guardrails.
type Request struct {
	Command string
	Args    []string
	Timeout time.Duration
}

// Result captures stdout and stderr from a sandboxed command.
type Result struct {
	Stdout string
	Stderr string
}

// Runner executes commands after policy approval and with a bounded timeout.
type Runner struct {
	Timeout time.Duration
	Policy  governance.Policy
}

// Run applies the policy check, enforces the timeout, and returns the command
// output without crashing the caller on failure.
func (r *Runner) Run(ctx context.Context, req Request) (Result, error) {
	decision := r.Policy.Decide(governance.CommandRequest{Command: req.Command, Source: "sandbox"})
	if decision.Action == governance.Deny {
		// Denied commands do not reach the host process layer.
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
	// CommandContext keeps the sandbox bounded even when the command hangs.
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
