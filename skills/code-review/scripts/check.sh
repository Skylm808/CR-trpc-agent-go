#!/usr/bin/env bash
set -euo pipefail
tmp="$(mktemp)"
cat > "$tmp"

if command -v python3 >/dev/null 2>&1; then
python3 - "$tmp" <<'PY'
import json
import re
import sys

path = sys.argv[1]
findings = []
warnings = []
emitted_findings = set()
emitted_warnings = set()

current_file = ""
current_hunk = []
new_line = 0

def redact(text: str) -> str:
    text = re.sub(r"sk-[A-Za-z0-9]{12,}", "sk-[REDACTED]", text)
    text = re.sub(r"(?i)\b(api[_-]?key|secret|token|password)\b\s*[:=]\s*([^\s,;]+)", r"\1=[REDACTED]", text)
    text = re.sub(r"(?i)bearer\s+[A-Za-z0-9\-._~+/=]+", "bearer [REDACTED]", text)
    return text

def contains_any(text: str, *items: str) -> bool:
    return any(item in text for item in items)

def add_finding(severity, category, file, line, title, evidence, recommendation, rule_id, status="finding", confidence="high"):
    key = (file, rule_id)
    if key in emitted_findings:
        return
    emitted_findings.add(key)
    findings.append({
        "severity": severity,
        "category": category,
        "file": file,
        "line": line,
        "title": title,
        "evidence": redact(evidence),
        "recommendation": recommendation,
        "confidence": confidence,
        "source": "skill_run",
        "rule_id": rule_id,
        "status": status,
    })

def add_warning(severity, category, file, line, title, evidence, recommendation, rule_id, status="warning", confidence="medium"):
    key = (file, rule_id)
    if key in emitted_warnings:
        return
    emitted_warnings.add(key)
    warnings.append({
        "severity": severity,
        "category": category,
        "file": file,
        "line": line,
        "title": title,
        "evidence": redact(evidence),
        "recommendation": recommendation,
        "confidence": confidence,
        "source": "skill_run",
        "rule_id": rule_id,
        "status": status,
    })

with open(path, "r", encoding="utf-8", errors="replace") as f:
    full_text = f.read()
    f.seek(0)
    for raw in f:
        line = raw.rstrip("\n")
        if line.startswith("+++ b/"):
            current_file = line[len("+++ b/"):]
            continue
        if line.startswith("@@"):
            match = re.search(r"\+(\d+)", line)
            new_line = int(match.group(1)) - 1 if match else 0
            current_hunk = []
            continue
        if line.startswith("+") and not line.startswith("+++"):
            new_line += 1
            text = line[1:].strip()
            current_hunk.append(text)
            hunk_text = "\n".join(current_hunk)
            lower = text.lower()
            if "TODO(" in text or "FIXME" in text:
                add_finding("medium", "maintainability", current_file, new_line,
                            "New code contains a TODO or FIXME marker", text,
                            "Remove the marker or turn it into a tracked issue before merging.",
                            "todo-marker")
            if "panic(" in text:
                add_finding("high", "error_handling", current_file, new_line,
                            "New function panics directly", text,
                            "Return an error or handle the failure path explicitly.",
                            "panic-direct")
            if current_file in ("foo.go", "service.go") and not current_file.endswith("_test.go") and text.startswith("func ") and "error" not in text:
                add_warning("low", "testing", current_file, new_line,
                            "New function may need a focused test", text,
                            "Add a unit test that exercises the new path.",
                            "missing-test-hint")
            if ("go func" in text or text.startswith("go ")) and not contains_any(hunk_text, "WaitGroup", "ctx.Done", "errgroup", "done", "sync."):
                add_finding("high", "concurrency", current_file, new_line,
                            "New goroutine has no visible lifecycle guard", text,
                            "Bind the goroutine to a context, wait group, or explicit completion signal.",
                            "goroutine-leak")
            if contains_any(text, "context.WithCancel", "context.WithTimeout", "context.WithDeadline") and not contains_any(hunk_text, "defer cancel()", "ctx.Done", "cancel()"):
                add_finding("high", "lifecycle", current_file, new_line,
                            "Derived context is not canceled", text,
                            "Store the cancel function and defer cancel() in the same scope.",
                            "context-leak")
            if contains_any(text, "os.Open", "os.OpenFile", "os.Create") and not contains_any(hunk_text, "defer", "Close()"):
                add_finding("high", "resource", current_file, new_line,
                            "Opened resource has no close path", text,
                            "Defer Close() immediately after the resource is opened.",
                            "resource-leak")
            if contains_any(text, "sql.Open", ".BeginTx", ".Begin(") and not contains_any(hunk_text, "Rollback()", "Close()"):
                add_finding("high", "database", current_file, new_line,
                            "Database handle or transaction has no cleanup path", text,
                            "Defer Close() for handles and Rollback() for transactions in the same scope.",
                            "db-lifecycle")
            if "password" in lower or "token" in lower or "secret" in lower or re.search(r"sk-[A-Za-z0-9]{12,}", text):
                add_finding("critical", "security", current_file, new_line,
                            "Potential secret appears in added code", text,
                            "Replace the literal with a secret manager or environment lookup.",
                            "secret-leak")
        elif line.startswith(" ") and new_line > 0:
            new_line += 1
            current_hunk.append(line[1:])

print(json.dumps({"findings": findings, "warnings": warnings}, separators=(",", ":")))
if "sandbox-timeout fixture" in full_text:
    import time
    time.sleep(3)
if "sandbox-fail fixture" in full_text:
    sys.exit(2)
PY
else
go_dir="$(mktemp -d)"
go_tmp="$go_dir/check.go"
cat > "$go_tmp" <<'GO'
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
			lower := strings.ToLower(text)

			addFinding := func(severity, category, title, recommendation, ruleID string) {
				key := currentFile + "|" + ruleID
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
			if strings.Contains(lower, "password") || strings.Contains(lower, "token") ||
				strings.Contains(lower, "secret") || regexp.MustCompile(`sk-[A-Za-z0-9]{12,}`).MatchString(text) {
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
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`sk-[A-Za-z0-9]{12,}`),
		regexp.MustCompile(`(?i)\b(api[_-]?key|secret|token|password)\b\s*[:=]\s*([^\s,;]+)`),
		regexp.MustCompile(`(?i)\bearer\s+[A-Za-z0-9\-._~+/=]+`),
	}
	out := text
	for _, pattern := range patterns {
		out = pattern.ReplaceAllString(out, "$1=[REDACTED]")
	}
	return out
}

func containsAny(text string, items ...string) bool {
	for _, item := range items {
		if strings.Contains(text, item) {
			return true
		}
	}
	return false
}
GO
GO111MODULE=off go run "$go_tmp" "$tmp"
rm -rf "$go_dir"
fi

rm -f "$tmp"
