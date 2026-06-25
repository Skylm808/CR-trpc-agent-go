package report

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
)

func BuildJSON(result review.Result) ([]byte, error) {
	result.Findings = review.DedupeFindings(result.Findings)
	return json.MarshalIndent(result, "", "  ")
}

func BuildMarkdown(result review.Result) string {
	findings := review.DedupeFindings(result.Findings)
	var b strings.Builder
	b.WriteString("# Review Report\n\n")
	if result.Summary != "" {
		b.WriteString(result.Summary)
		b.WriteString("\n\n")
	}
	fmt.Fprintf(&b, "Findings: %d\n\n", len(findings))
	for _, f := range findings {
		fmt.Fprintf(&b, "- [%s] %s:%d %s\n", strings.ToUpper(f.Severity), f.File, f.Line, f.Title)
		if f.Evidence != "" {
			fmt.Fprintf(&b, "  - Evidence: %s\n", f.Evidence)
		}
		if f.Recommendation != "" {
			fmt.Fprintf(&b, "  - Recommendation: %s\n", f.Recommendation)
		}
	}
	return b.String()
}

