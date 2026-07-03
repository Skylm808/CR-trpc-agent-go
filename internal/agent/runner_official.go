package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
	officialagent "trpc.group/trpc-go/trpc-agent-go/agent"
	agentevent "trpc.group/trpc-go/trpc-agent-go/event"
	agentmodel "trpc.group/trpc-go/trpc-agent-go/model"
	agentrunner "trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

const officialReviewAgentName = "cr-agent"

// RunWithEvents executes a review through the official runner and returns its event stream.
func (a *Agent) RunWithEvents(ctx context.Context, req Request) (<-chan *agentevent.Event, error) {
	if a == nil {
		return nil, fmt.Errorf("agent is required")
	}
	adapter := reviewRunnerAgent{base: a, req: req}
	r := agentrunner.NewRunner("cr-agent", adapter, agentrunner.WithArtifactService(a.artifactService))
	sessionID := fmt.Sprintf("review-%d", time.Now().UnixNano())
	events, err := r.Run(
		ctx,
		"local",
		sessionID,
		agentmodel.NewUserMessage("run code review"),
		officialagent.WithRequestID(sessionID),
	)
	if err != nil {
		_ = r.Close()
		return nil, err
	}
	out := make(chan *agentevent.Event)
	go func() {
		defer close(out)
		defer r.Close()
		for ev := range events {
			out <- ev
		}
	}()
	return out, nil
}

type reviewRunnerAgent struct {
	base *Agent
	req  Request
}

func (a reviewRunnerAgent) Run(ctx context.Context, invocation *officialagent.Invocation) (<-chan *agentevent.Event, error) {
	if a.base == nil {
		return nil, fmt.Errorf("agent is required")
	}
	events := make(chan *agentevent.Event, 16)
	local := *a.base
	originalSink := local.cfg.EventSink
	local.cfg.EventSink = func(ctx context.Context, ev *agentevent.Event) {
		_ = ctx
		if ev == nil {
			return
		}
		if originalSink != nil {
			originalSink(ctx, ev.Clone())
		}
		officialagent.InjectIntoEvent(invocation, ev)
		events <- ev
	}
	go func() {
		defer close(events)
		if _, err := local.runDirect(ctx, a.req); err != nil {
			ev := agentevent.NewErrorEvent("", officialReviewAgentName, "run_error", review.RedactSecrets(err.Error()))
			officialagent.InjectIntoEvent(invocation, ev)
			events <- ev
		}
	}()
	_ = invocation
	return events, nil
}

func (a reviewRunnerAgent) Tools() []tool.Tool {
	return nil
}

func (a reviewRunnerAgent) Info() officialagent.Info {
	return officialagent.Info{
		Name:        officialReviewAgentName,
		Description: "Runs the CR Agent review pipeline and emits official review events.",
	}
}

func (a reviewRunnerAgent) SubAgents() []officialagent.Agent {
	return nil
}

func (a reviewRunnerAgent) FindSubAgent(name string) officialagent.Agent {
	_ = name
	return nil
}
