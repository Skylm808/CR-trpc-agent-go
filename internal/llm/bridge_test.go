package llm

import (
	"strings"
	"testing"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
)

func TestDecodeLLMOutputAcceptsPlainJSON(t *testing.T) {
	output, err := DecodeOutput(`{"findings":[{"rule_id":"plain-json","confidence":"high"}]}`)
	if err != nil {
		t.Fatalf("decode plain JSON: %v", err)
	}
	if !hasRuleID(output.Findings, "plain-json") {
		t.Fatalf("expected plain JSON finding, got %+v", output.Findings)
	}
}

func TestDecodeLLMOutputAcceptsFencedJSON(t *testing.T) {
	output, err := DecodeOutput("```json\n{\"findings\":[{\"rule_id\":\"fenced-json\",\"confidence\":\"low\"}]}\n```")
	if err != nil {
		t.Fatalf("decode fenced JSON: %v", err)
	}
	if !hasRuleID(output.Findings, "fenced-json") {
		t.Fatalf("expected fenced JSON finding, got %+v", output.Findings)
	}
}

func TestDecodeLLMOutputExtractsJSONFromText(t *testing.T) {
	output, err := DecodeOutput("Review result:\n{\"findings\":[{\"rule_id\":\"embedded-json\",\"confidence\":\"medium\"}]}\nDone.")
	if err != nil {
		t.Fatalf("decode embedded JSON: %v", err)
	}
	if !hasRuleID(output.Findings, "embedded-json") {
		t.Fatalf("expected embedded JSON finding, got %+v", output.Findings)
	}
}

func TestDecodeLLMOutputEmptyContent(t *testing.T) {
	output, err := DecodeOutput("  ")
	if err != nil {
		t.Fatalf("decode empty content: %v", err)
	}
	if len(output.Findings) != 0 {
		t.Fatalf("expected empty output, got %+v", output)
	}
}

func TestDecodeLLMOutputRedactsInvalidJSONError(t *testing.T) {
	_, err := DecodeOutput(`{"findings":[{"evidence":"sk-invalidjson-1234567890abcdef"}`)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if strings.Contains(err.Error(), "sk-invalidjson-1234567890abcdef") {
		t.Fatalf("decode error leaked secret: %v", err)
	}
}

func TestModelReviewSystemPromptDefinesStrictContract(t *testing.T) {
	req := InputRequest(Input{})
	if len(req.Messages) == 0 {
		t.Fatal("expected system prompt")
	}
	prompt := req.Messages[0].Content
	for _, want := range []string{
		"only return a JSON object",
		"do not return markdown",
		`"findings"`,
		"severity",
		"confidence",
		"high, medium, or low",
		"do not duplicate existing_findings",
		"Only report incremental semantic value",
		"cross-file",
		"business logic",
		"Return an empty findings array",
		"Do not output secrets",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, prompt)
		}
	}
}

func hasRuleID(findings []review.Finding, ruleID string) bool {
	for _, finding := range findings {
		if finding.RuleID == ruleID {
			return true
		}
	}
	return false
}
