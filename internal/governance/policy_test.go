package governance

import "testing"

func TestPolicyDeniesHighRiskShellCommands(t *testing.T) {
	policy := DefaultPolicy()
	decision := policy.Decide(CommandRequest{
		Command: "rm -rf /",
		Source:  "sandbox",
	})
	if decision.Action != Deny {
		t.Fatalf("expected deny, got %v", decision.Action)
	}
}

func TestPolicyAllowsGoVet(t *testing.T) {
	policy := DefaultPolicy()
	decision := policy.Decide(CommandRequest{
		Command: "go vet ./...",
		Source:  "sandbox",
	})
	if decision.Action != Allow {
		t.Fatalf("expected allow, got %v", decision.Action)
	}
}

