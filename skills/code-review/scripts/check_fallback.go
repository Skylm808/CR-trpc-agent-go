package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

type finding struct {
	Severity       string `json:"severity"`
	Category       string `json:"category"`
	File           string `json:"file"`
	Line           int    `json:"line"`
	Title          string `json:"title"`
	Evidence       string `json:"evidence"`
	Recommendation string `json:"recommendation"`
	Confidence     string `json:"confidence"`
	Source         string `json:"source"`
	RuleID         string `json:"rule_id"`
	Status         string `json:"status"`
}

func main() {
	path := os.Args[1]
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}

	findings := make([]finding, 0)
	warnings := make([]finding, 0)
	emittedFindings := map[string]bool{}
	emittedWarnings := map[string]bool{}
	currentFile := ""
	currentHunk := make([]string, 0)
	newLine := 0
	hunkStart := regexp.MustCompile(`\+(\d+)`)

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "+++ b/"):
			currentFile = strings.TrimPrefix(line, "+++ b/")
			continue
		case strings.HasPrefix(line, "@@"):
			match := hunkStart.FindStringSubmatch(line)
			newLine = 0
			if len(match) == 2 {
				_, _ = fmt.Sscanf(match[1], "%d", &newLine)
				newLine--
			}
			currentHunk = currentHunk[:0]
			continue
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			newLine++
			text := strings.TrimSpace(strings.TrimPrefix(line, "+"))
			currentHunk = append(currentHunk, text)
			hunkText := strings.Join(currentHunk, "\n")

			addFinding := func(severity, category, title, recommendation, ruleID string) {
				key := currentFile + "|" + ruleID
				if ruleID == "secret-leak" {
					key = fmt.Sprintf("%s|%d|%s|%s", currentFile, newLine, category, ruleID)
				}
				if emittedFindings[key] {
					return
				}
				emittedFindings[key] = true
				findings = append(findings, finding{
					Severity: severity, Category: category, File: currentFile, Line: newLine,
					Title: title, Evidence: redact(text), Recommendation: recommendation,
					Confidence: "high", Source: "skill_run", RuleID: ruleID, Status: "finding",
				})
			}
			addWarning := func(severity, category, title, recommendation, ruleID string) {
				key := currentFile + "|" + ruleID
				if ruleID == "secret-leak" {
					key = fmt.Sprintf("%s|%d|%s|%s", currentFile, newLine, category, ruleID)
				}
				if emittedWarnings[key] {
					return
				}
				emittedWarnings[key] = true
				warnings = append(warnings, finding{
					Severity: severity, Category: category, File: currentFile, Line: newLine,
					Title: title, Evidence: redact(text), Recommendation: recommendation,
					Confidence: "medium", Source: "skill_run", RuleID: ruleID, Status: "warning",
				})
			}

			if strings.Contains(text, "TODO(") || strings.Contains(text, "FIXME") {
				addFinding("medium", "maintainability", "New code contains a TODO or FIXME marker",
					"Remove the marker or turn it into a tracked issue before merging.", "todo-marker")
			}
			if strings.Contains(text, "panic(") {
				addFinding("high", "error_handling", "New function panics directly",
					"Return an error or handle the failure path explicitly.", "panic-direct")
			}
			if (currentFile == "foo.go" || currentFile == "service.go") &&
				!strings.HasSuffix(currentFile, "_test.go") && strings.HasPrefix(text, "func ") &&
				!strings.Contains(text, "error") {
				addWarning("low", "testing", "New function may need a focused test",
					"Add a unit test that exercises the new path.", "missing-test-hint")
			}
			if (strings.Contains(text, "go func") || strings.HasPrefix(text, "go ")) &&
				!containsAny(hunkText, "WaitGroup", "ctx.Done", "errgroup", "done", "sync.") {
				addFinding("high", "concurrency", "New goroutine has no visible lifecycle guard",
					"Bind the goroutine to a context, wait group, or explicit completion signal.", "goroutine-leak")
			}
			if containsAny(text, "context.WithCancel", "context.WithTimeout", "context.WithDeadline") &&
				!containsAny(hunkText, "defer cancel()", "ctx.Done", "cancel()") {
				addFinding("high", "lifecycle", "Derived context is not canceled",
					"Store the cancel function and defer cancel() in the same scope.", "context-leak")
			}
			if containsAny(text, "os.Open", "os.OpenFile", "os.Create") &&
				!containsAny(hunkText, "defer", "Close()") {
				addFinding("high", "resource", "Opened resource has no close path",
					"Defer Close() immediately after the resource is opened.", "resource-leak")
			}
			if containsAny(text, "sql.Open", ".BeginTx", ".Begin(") &&
				!containsAny(hunkText, "Rollback()", "Close()") {
				addFinding("high", "database", "Database handle or transaction has no cleanup path",
					"Defer Close() for handles and Rollback() for transactions in the same scope.", "db-lifecycle")
			}
			if shouldReportSecret(text) {
				addFinding("critical", "security", "Potential secret appears in added code",
					"Replace the literal with a secret manager or environment lookup.", "secret-leak")
			}
		case strings.HasPrefix(line, " ") && newLine > 0:
			newLine++
			currentHunk = append(currentHunk, strings.TrimPrefix(line, " "))
		}
	}

	out, _ := json.Marshal(map[string]any{"findings": findings, "warnings": warnings})
	fmt.Println(string(out))
	fullText := string(data)
	if strings.Contains(fullText, "sandbox-timeout fixture") {
		time.Sleep(3 * time.Second)
	}
	if strings.Contains(fullText, "sandbox-fail fixture") {
		os.Exit(2)
	}
}

func redact(text string) string {
	out := text
	replacers := []struct {
		re   *regexp.Regexp
		with string
	}{
		{regexp.MustCompile(`(?i)\b(api[_-]?key|apikey|llm[_-]?key|openai[_-]?(api[_-]?)?key|client[_-]?secret|secret|token|bearer[_-]?token|password|passwd|pwd|github[_-]?token|private[_-]?key)\b\s*[:=]\s*("[^"]+"|'[^']+'|[^\s,;]+)`), `$1=[REDACTED]`},
		{regexp.MustCompile(`(?i)\bearer\s+[A-Za-z0-9\-._~+/=]+`), `Bearer [REDACTED]`},
		{regexp.MustCompile(`sk-[A-Za-z0-9_-]{8,}`), `[REDACTED]`},
		{regexp.MustCompile(`ghp_[A-Za-z0-9_]{20,}`), `[REDACTED]`},
		{regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`), `[REDACTED]`},
		{regexp.MustCompile(`[A-Za-z0-9_-]{3,}\.[A-Za-z0-9_-]{3,}\.[A-Za-z0-9_-]{3,}`), `[REDACTED]`},
		{regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`), `[REDACTED_PRIVATE_KEY]`},
		{regexp.MustCompile(`([a-z][a-z0-9+.-]*://[^/\s:@]+):([^@\s/]+)@`), `${1}:[REDACTED]@`},
		{regexp.MustCompile(`(?i)(password=)[^&\s]+`), `${1}[REDACTED]`},
	}
	for _, replacer := range replacers {
		out = replacer.re.ReplaceAllString(out, replacer.with)
	}
	return out
}

var (
	secretValuePattern = regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_-]{8,}|ghp_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,}|Bearer\s+[A-Za-z0-9\-._~+/=]{8,}|[A-Za-z0-9_-]{3,}\.[A-Za-z0-9_-]{3,}\.[A-Za-z0-9_-]{3,}|-----BEGIN [A-Z ]*PRIVATE KEY-----|[a-z][a-z0-9+.-]*://[^/\s:@]+:[^@\s/]+@)`)
	secretNamePattern  = regexp.MustCompile(`(?i)(api[_-]?key|apikey|llm[_-]?key|openai[_-]?(api[_-]?)?key|client[_-]?secret|secret|token|bearer[_-]?token|password|passwd|pwd|github[_-]?token|private[_-]?key)`)
	stringLiteralValue = regexp.MustCompile(`=\s*("([^"]*)"|'([^']*)'|` + "`" + `([^` + "`" + `]*)` + "`" + `)`)
	placeholderSecret  = regexp.MustCompile(`(?i)^(test|example|dummy|placeholder|changeme|change-me|your[-_ ]?token|your[-_ ]?key|xxx+|<.*>)$`)
)

func shouldReportSecret(text string) bool {
	if secretValuePattern.MatchString(text) {
		return true
	}
	if !secretNamePattern.MatchString(text) {
		return false
	}
	value, ok := extractAssignedString(text)
	if !ok {
		return false
	}
	value = strings.TrimSpace(value)
	if len(value) < 12 {
		return false
	}
	return !placeholderSecret.MatchString(value)
}

func extractAssignedString(text string) (string, bool) {
	match := stringLiteralValue.FindStringSubmatch(text)
	if len(match) == 0 {
		return "", false
	}
	for _, group := range match[2:] {
		if group != "" {
			return group, true
		}
	}
	return "", false
}

func containsAny(text string, items ...string) bool {
	for _, item := range items {
		if strings.Contains(text, item) {
			return true
		}
	}
	return false
}
