package agent

import (
	"context"
	"fmt"
	"time"

	agentevent "trpc.group/trpc-go/trpc-agent-go/event"
	agentmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const (
	reviewEventInputLoaded   = "cr_agent.input_loaded"
	reviewEventSkillRun      = "cr_agent.skill_run"
	reviewEventSandboxRun    = "cr_agent.sandbox_run"
	reviewEventModelReview   = "cr_agent.model_review"
	reviewEventReportWritten = "cr_agent.report_written"
	reviewEventTaskFinished  = "cr_agent.task_finished"
	reviewEventTaskFailed    = "cr_agent.task_failed"
)

func (a *Agent) emitReviewEvent(ctx context.Context, taskID, object, content string) {
	if a == nil || a.cfg.EventSink == nil {
		return
	}
	now := time.Now()
	a.cfg.EventSink(ctx, &agentevent.Event{
		Response: &agentmodel.Response{
			Object:  object,
			Created: now.Unix(),
			Model:   "cr-agent",
			Choices: []agentmodel.Choice{{
				Index: 0,
				Message: agentmodel.Message{
					Role:    agentmodel.RoleAssistant,
					Content: content,
				},
			}},
			Done: true,
		},
		InvocationID: taskID,
		Author:       "cr-agent",
		ID:           fmt.Sprintf("%s:%s:%d", taskID, object, now.UnixNano()),
		Timestamp:    now,
	})
}
