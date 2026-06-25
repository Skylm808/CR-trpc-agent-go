package sandbox

import (
	"context"
	"testing"
	"time"
)

func TestRunnerFailsClosedOnTimeout(t *testing.T) {
	r := &Runner{
		Timeout: 10 * time.Millisecond,
	}
	_, err := r.Run(context.Background(), Request{
		Command: "sleep 1",
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

