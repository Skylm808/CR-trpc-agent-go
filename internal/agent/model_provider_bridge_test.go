package agent

import (
	"strings"
	"testing"
)

func TestDecodeModelReviewOutputAcceptsPlainJSON(t *testing.T) {
	output, err := decodeModelReviewOutput(`{"findings":[{"rule_id":"plain-json","confidence":"high"}]}`)
	if err != nil {
		t.Fatalf("decode plain JSON: %v", err)
	}
	if !hasRuleID(output.Findings, "plain-json") {
		t.Fatalf("expected plain JSON finding, got %+v", output.Findings)
	}
}

func TestDecodeModelReviewOutputAcceptsFencedJSON(t *testing.T) {
	output, err := decodeModelReviewOutput("```json\n{\"findings\":[{\"rule_id\":\"fenced-json\",\"confidence\":\"low\"}]}\n```")
	if err != nil {
		t.Fatalf("decode fenced JSON: %v", err)
	}
	if !hasRuleID(output.Findings, "fenced-json") {
		t.Fatalf("expected fenced JSON finding, got %+v", output.Findings)
	}
}

func TestDecodeModelReviewOutputExtractsJSONFromText(t *testing.T) {
	output, err := decodeModelReviewOutput("Review result:\n{\"findings\":[{\"rule_id\":\"embedded-json\",\"confidence\":\"medium\"}]}\nDone.")
	if err != nil {
		t.Fatalf("decode embedded JSON: %v", err)
	}
	if !hasRuleID(output.Findings, "embedded-json") {
		t.Fatalf("expected embedded JSON finding, got %+v", output.Findings)
	}
}

func TestDecodeModelReviewOutputEmptyContent(t *testing.T) {
	output, err := decodeModelReviewOutput("  ")
	if err != nil {
		t.Fatalf("decode empty content: %v", err)
	}
	if len(output.Findings) != 0 {
		t.Fatalf("expected empty output, got %+v", output)
	}
}

func TestDecodeModelReviewOutputRedactsInvalidJSONError(t *testing.T) {
	_, err := decodeModelReviewOutput(`{"findings":[{"evidence":"sk-invalidjson-1234567890abcdef"}`)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if strings.Contains(err.Error(), "sk-invalidjson-1234567890abcdef") {
		t.Fatalf("decode error leaked secret: %v", err)
	}
}

func TestModelReviewSystemPromptDefinesStrictContract(t *testing.T) {
	req := modelReviewInputRequest(ModelReviewInput{})
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
		"Do not output secrets",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, prompt)
		}
	}
}
