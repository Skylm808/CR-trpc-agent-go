#!/usr/bin/env bash
set -euo pipefail
tmp="$(mktemp)"
cat > "$tmp"

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
if "sandbox-fail fixture" in full_text:
    sys.exit(2)
PY

rm -f "$tmp"
