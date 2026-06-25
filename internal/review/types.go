package review

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Result struct {
	TaskID   string     `json:"task_id"`
	Findings []Finding  `json:"findings"`
	Warnings []Finding  `json:"warnings,omitempty"`
	Metrics  Metrics    `json:"metrics,omitempty"`
	Summary  string     `json:"summary,omitempty"`
	Created  time.Time  `json:"created_at,omitempty"`
}

type Metrics struct {
	TotalDurationMS    int64            `json:"total_duration_ms,omitempty"`
	SandboxDurationMS  int64            `json:"sandbox_duration_ms,omitempty"`
	ToolCallCount      int              `json:"tool_call_count,omitempty"`
	PermissionBlocks   int              `json:"permission_block_count,omitempty"`
	FindingCount       int              `json:"finding_count,omitempty"`
	SeverityCounts     map[string]int   `json:"severity_counts,omitempty"`
	ExceptionCounts    map[string]int   `json:"exception_counts,omitempty"`
	RedactionCount     int              `json:"redaction_count,omitempty"`
}

type Finding struct {
	Severity       string `json:"severity"`
	Category       string `json:"category"`
	File           string `json:"file"`
	Line           int    `json:"line"`
	Title          string `json:"title"`
	Evidence       string `json:"evidence,omitempty"`
	Recommendation  string `json:"recommendation,omitempty"`
	Confidence     string `json:"confidence,omitempty"`
	Source         string `json:"source"`
	RuleID         string `json:"rule_id"`
	Status         string `json:"status,omitempty"`
}

func (f Finding) DedupeKey() string {
	sum := sha1.Sum([]byte(strings.Join([]string{
		strings.ToLower(strings.TrimSpace(f.File)),
		fmt.Sprintf("%d", f.Line),
		strings.ToLower(strings.TrimSpace(f.Category)),
		strings.ToLower(strings.TrimSpace(f.RuleID)),
	}, "|")))
	return hex.EncodeToString(sum[:])
}

type ParsedDiff struct {
	Files []ParsedFile `json:"files"`
}

type ParsedFile struct {
	Path        string  `json:"path"`
	Language    string  `json:"language"`
	PackageName string  `json:"package_name,omitempty"`
	IsTestFile  bool    `json:"is_test_file"`
	ChangeType  string  `json:"change_type,omitempty"`
	Hunks       []Hunk  `json:"hunks"`
}

type Hunk struct {
	File          string   `json:"file"`
	OldStart      int      `json:"old_start"`
	OldLines      int      `json:"old_lines"`
	NewStart      int      `json:"new_start"`
	NewLines      int      `json:"new_lines"`
	Context       []string `json:"context,omitempty"`
	CandidateLines []int   `json:"candidate_lines,omitempty"`
	Lines         []Line   `json:"lines,omitempty"`
}

type Line struct {
	OldLine int    `json:"old_line,omitempty"`
	NewLine int    `json:"new_line,omitempty"`
	Kind    string `json:"kind"`
	Text    string `json:"text"`
}

func RedactSecrets(input string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(api[_-]?key|secret|token|password)\b\s*[:=]\s*([^\s,;]+)`),
		regexp.MustCompile(`sk-[A-Za-z0-9]{12,}`),
		regexp.MustCompile(`(?i)\bearer\s+[A-Za-z0-9\-._~+/=]+`),
	}
	out := input
	for _, re := range patterns {
		out = re.ReplaceAllStringFunc(out, func(s string) string {
			return re.ReplaceAllString(s, "$1=[REDACTED]")
		})
	}
	return out
}

func DedupeFindings(findings []Finding) []Finding {
	seen := map[string]struct{}{}
	out := make([]Finding, 0, len(findings))
	for _, f := range findings {
		key := f.DedupeKey()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].RuleID < out[j].RuleID
	})
	return out
}

func MustJSON(v any) []byte {
	b, _ := json.MarshalIndent(v, "", "  ")
	return b
}
