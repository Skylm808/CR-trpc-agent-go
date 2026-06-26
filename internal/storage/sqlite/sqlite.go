// Package sqlite persists review tasks, findings, telemetry, and reports in a
// single-file SQLite database for the first version of the agent.
package sqlite

import (
	"context"
	"database/sql"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
)

// Store owns the SQLite connection used by the prototype.
type Store struct {
	db *sql.DB
}

// Task is the canonical persisted review task record.
type Task struct {
	ID          string
	InputType   string
	InputRef    string
	InputDigest string
	RepoPath    string
	Status      string
	Mode        string
	CreatedAt   time.Time
	StartedAt   time.Time
	FinishedAt  time.Time
}

// Report stores the generated JSON and Markdown report bodies.
type Report struct {
	JSON      []byte
	Markdown  []byte
	CreatedAt time.Time
}

// DecisionRecord captures one permission-policy decision for auditability.
type DecisionRecord struct {
	TaskID  string
	Command string
	Action  string
	Reason  string
	At      time.Time
}

// SandboxRunRecord captures one sandbox execution attempt.
type SandboxRunRecord struct {
	TaskID           string
	Command          string
	Runtime          string
	Status           string
	TimeoutMS        int64
	OutputLimitBytes int
	EnvWhitelist     string
	ExitCode         int
	StdoutDigest     string
	StderrDigest     string
	DurationMS       int64
	Output           string
	At               time.Time
}

// FilterDecisionRecord captures one filter or redaction decision.
type FilterDecisionRecord struct {
	TaskID string
	Target string
	Action string
	Reason string
	At     time.Time
}

// ArtifactRecord captures one persisted review artifact reference.
type ArtifactRecord struct {
	TaskID string
	Name   string
	Kind   string
	Path   string
	Digest string
	At     time.Time
}

// MetricsRecord stores the aggregated review telemetry for a task.
type MetricsRecord struct {
	TaskID               string
	TotalDurationMS      int64
	SandboxDurationMS    int64
	ToolCallCount        int
	PermissionBlockCount int
	FindingCount         int
	SeverityCountsJSON   string
	ExceptionCountsJSON  string
	RedactionCount       int
	At                   time.Time
}

// MetricsSummary is the query shape returned by MetricsByTaskID.
type MetricsSummary struct {
	TaskID               string
	TotalDurationMS      int64
	SandboxDurationMS    int64
	ToolCallCount        int
	PermissionBlockCount int
	FindingCount         int
	SeverityCountsJSON   string
	ExceptionCountsJSON  string
	RedactionCount       int
	At                   time.Time
}

// Open creates or opens the SQLite database at the provided path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.Init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Init creates the tables needed by the first-version review agent.
func (s *Store) Init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS review_tasks (
  task_id TEXT PRIMARY KEY,
  input_type TEXT NOT NULL,
  input_ref TEXT NOT NULL,
  input_digest TEXT NOT NULL,
  repo_path TEXT NOT NULL,
  status TEXT NOT NULL,
  mode TEXT NOT NULL,
  created_at TEXT NOT NULL,
  started_at TEXT,
  finished_at TEXT
);
CREATE TABLE IF NOT EXISTS findings (
  finding_id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  severity TEXT NOT NULL,
  category TEXT NOT NULL,
  file TEXT NOT NULL,
  line INTEGER NOT NULL,
  title TEXT NOT NULL,
  evidence TEXT,
  recommendation TEXT,
  confidence TEXT,
  source TEXT NOT NULL,
  rule_id TEXT NOT NULL,
  dedupe_key TEXT NOT NULL,
  status TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS reports (
  task_id TEXT PRIMARY KEY,
  json_report BLOB NOT NULL,
  markdown_report BLOB NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS permission_decisions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id TEXT NOT NULL,
  command TEXT NOT NULL,
  action TEXT NOT NULL,
  reason TEXT,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS filter_decisions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id TEXT NOT NULL,
  target TEXT NOT NULL,
  action TEXT NOT NULL,
  reason TEXT,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sandbox_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id TEXT NOT NULL,
  command TEXT NOT NULL,
  runtime TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  timeout_ms INTEGER NOT NULL DEFAULT 0,
  output_limit_bytes INTEGER NOT NULL DEFAULT 0,
  env_whitelist TEXT NOT NULL DEFAULT '',
  exit_code INTEGER NOT NULL DEFAULT 0,
  stdout_digest TEXT NOT NULL DEFAULT '',
  stderr_digest TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0,
  output TEXT,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS artifacts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id TEXT NOT NULL,
  name TEXT NOT NULL,
  kind TEXT NOT NULL,
  path TEXT,
  digest TEXT,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS metrics (
  task_id TEXT PRIMARY KEY,
  total_duration_ms INTEGER NOT NULL,
  sandbox_duration_ms INTEGER NOT NULL,
  tool_call_count INTEGER NOT NULL,
  permission_block_count INTEGER NOT NULL,
  finding_count INTEGER NOT NULL,
  severity_counts_json TEXT NOT NULL,
  exception_counts_json TEXT NOT NULL,
  redaction_count INTEGER NOT NULL,
  created_at TEXT NOT NULL
);
`)
	return err
}

// Close closes the owned database handle.
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// SaveTask inserts or updates the task row used as the persistence anchor.
func (s *Store) SaveTask(ctx context.Context, task Task) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO review_tasks(task_id, input_type, input_ref, input_digest, repo_path, status, mode, created_at, started_at, finished_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(task_id) DO UPDATE SET
input_type=excluded.input_type,
input_ref=excluded.input_ref,
input_digest=excluded.input_digest,
repo_path=excluded.repo_path,
status=excluded.status,
mode=excluded.mode,
created_at=excluded.created_at,
started_at=excluded.started_at,
finished_at=excluded.finished_at
`,
		task.ID, task.InputType, task.InputRef, task.InputDigest, task.RepoPath, task.Status, task.Mode,
		task.CreatedAt.UTC().Format(time.RFC3339Nano), nullableTime(task.StartedAt), nullableTime(task.FinishedAt))
	return err
}

// SaveFinding writes one structured finding row.
func (s *Store) SaveFinding(ctx context.Context, taskID string, finding review.Finding) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO findings(finding_id, task_id, severity, category, file, line, title, evidence, recommendation, confidence, source, rule_id, dedupe_key, status)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`,
		finding.DedupeKey(), taskID, finding.Severity, finding.Category, finding.File, finding.Line, finding.Title,
		finding.Evidence, finding.Recommendation, finding.Confidence, finding.Source, finding.RuleID, finding.DedupeKey(), finding.Status)
	return err
}

// SaveReport writes the final JSON and Markdown artifacts.
func (s *Store) SaveReport(ctx context.Context, taskID string, jsonReport, markdownReport []byte) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO reports(task_id, json_report, markdown_report, created_at)
VALUES(?, ?, ?, ?)
ON CONFLICT(task_id) DO UPDATE SET
json_report=excluded.json_report,
markdown_report=excluded.markdown_report,
created_at=excluded.created_at
`,
		taskID, jsonReport, markdownReport, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

// TaskByID loads a task for later inspection or replay.
func (s *Store) TaskByID(ctx context.Context, id string) (Task, error) {
	var task Task
	var createdAt string
	var startedAt, finishedAt sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT task_id, input_type, input_ref, input_digest, repo_path, status, mode, created_at, started_at, finished_at
FROM review_tasks WHERE task_id=?
`, id).Scan(&task.ID, &task.InputType, &task.InputRef, &task.InputDigest, &task.RepoPath, &task.Status, &task.Mode, &createdAt, &startedAt, &finishedAt)
	if err != nil {
		return Task{}, err
	}
	task.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	task.StartedAt = parseNullableTime(startedAt)
	task.FinishedAt = parseNullableTime(finishedAt)
	return task, nil
}

// FindingsByTaskID loads the stored findings for one task.
func (s *Store) FindingsByTaskID(ctx context.Context, taskID string) ([]review.Finding, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT severity, category, file, line, title, evidence, recommendation, confidence, source, rule_id, status
FROM findings WHERE task_id=?
ORDER BY file, line, rule_id
`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []review.Finding
	for rows.Next() {
		var f review.Finding
		if err := rows.Scan(&f.Severity, &f.Category, &f.File, &f.Line, &f.Title, &f.Evidence, &f.Recommendation, &f.Confidence, &f.Source, &f.RuleID, &f.Status); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ReportByTaskID loads the persisted report bodies for one task.
func (s *Store) ReportByTaskID(ctx context.Context, taskID string) (Report, error) {
	var rep Report
	var createdAt string
	err := s.db.QueryRowContext(ctx, `
SELECT json_report, markdown_report, created_at FROM reports WHERE task_id=?
`, taskID).Scan(&rep.JSON, &rep.Markdown, &createdAt)
	if err != nil {
		return Report{}, err
	}
	rep.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return rep, nil
}

// SaveDecision writes a governance decision for the audit trail.
func (s *Store) SaveDecision(ctx context.Context, rec DecisionRecord) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO permission_decisions(task_id, command, action, reason, created_at)
VALUES(?, ?, ?, ?, ?)
`, rec.TaskID, rec.Command, rec.Action, rec.Reason, rec.At.UTC().Format(time.RFC3339Nano))
	return err
}

// SaveSandboxRun writes one sandbox attempt for later troubleshooting.
func (s *Store) SaveSandboxRun(ctx context.Context, rec SandboxRunRecord) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO sandbox_runs(task_id, command, runtime, status, timeout_ms, output_limit_bytes, env_whitelist, exit_code, stdout_digest, stderr_digest, duration_ms, output, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, rec.TaskID, rec.Command, rec.Runtime, rec.Status, rec.TimeoutMS, rec.OutputLimitBytes, rec.EnvWhitelist, rec.ExitCode, rec.StdoutDigest, rec.StderrDigest, rec.DurationMS, rec.Output, rec.At.UTC().Format(time.RFC3339Nano))
	return err
}

// SaveFilterDecision writes one filter or redaction decision for auditability.
func (s *Store) SaveFilterDecision(ctx context.Context, rec FilterDecisionRecord) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO filter_decisions(task_id, target, action, reason, created_at)
VALUES(?, ?, ?, ?, ?)
`, rec.TaskID, rec.Target, rec.Action, rec.Reason, rec.At.UTC().Format(time.RFC3339Nano))
	return err
}

// SaveArtifact writes one persisted artifact reference for a task.
func (s *Store) SaveArtifact(ctx context.Context, rec ArtifactRecord) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO artifacts(task_id, name, kind, path, digest, created_at)
VALUES(?, ?, ?, ?, ?, ?)
`, rec.TaskID, rec.Name, rec.Kind, rec.Path, rec.Digest, rec.At.UTC().Format(time.RFC3339Nano))
	return err
}

// DecisionsByTaskID 按写入顺序加载某个任务的权限决策记录。
func (s *Store) DecisionsByTaskID(ctx context.Context, taskID string) ([]DecisionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT task_id, command, action, reason, created_at
FROM permission_decisions WHERE task_id=?
ORDER BY id
`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DecisionRecord
	for rows.Next() {
		var rec DecisionRecord
		var createdAt string
		if err := rows.Scan(&rec.TaskID, &rec.Command, &rec.Action, &rec.Reason, &createdAt); err != nil {
			return nil, err
		}
		rec.At, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, rec)
	}
	return out, rows.Err()
}

// SandboxRunsByTaskID 按写入顺序加载某个任务的沙箱执行记录。
func (s *Store) SandboxRunsByTaskID(ctx context.Context, taskID string) ([]SandboxRunRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT task_id, command, runtime, status, timeout_ms, output_limit_bytes, env_whitelist, exit_code, stdout_digest, stderr_digest, duration_ms, output, created_at
FROM sandbox_runs WHERE task_id=?
ORDER BY id
`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SandboxRunRecord
	for rows.Next() {
		var rec SandboxRunRecord
		var createdAt string
		if err := rows.Scan(&rec.TaskID, &rec.Command, &rec.Runtime, &rec.Status, &rec.TimeoutMS, &rec.OutputLimitBytes, &rec.EnvWhitelist, &rec.ExitCode, &rec.StdoutDigest, &rec.StderrDigest, &rec.DurationMS, &rec.Output, &createdAt); err != nil {
			return nil, err
		}
		rec.At, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, rec)
	}
	return out, rows.Err()
}

// FilterDecisionsByTaskID 按写入顺序加载某个任务的过滤/脱敏决策。
func (s *Store) FilterDecisionsByTaskID(ctx context.Context, taskID string) ([]FilterDecisionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT task_id, target, action, reason, created_at
FROM filter_decisions WHERE task_id=?
ORDER BY id
`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FilterDecisionRecord
	for rows.Next() {
		var rec FilterDecisionRecord
		var createdAt string
		if err := rows.Scan(&rec.TaskID, &rec.Target, &rec.Action, &rec.Reason, &createdAt); err != nil {
			return nil, err
		}
		rec.At, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, rec)
	}
	return out, rows.Err()
}

// ArtifactsByTaskID 按写入顺序加载某个任务的持久化产物。
func (s *Store) ArtifactsByTaskID(ctx context.Context, taskID string) ([]ArtifactRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT task_id, name, kind, path, digest, created_at
FROM artifacts WHERE task_id=?
ORDER BY id
`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ArtifactRecord
	for rows.Next() {
		var rec ArtifactRecord
		var createdAt string
		if err := rows.Scan(&rec.TaskID, &rec.Name, &rec.Kind, &rec.Path, &rec.Digest, &createdAt); err != nil {
			return nil, err
		}
		rec.At, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, rec)
	}
	return out, rows.Err()
}

// SaveMetrics stores the aggregated telemetry snapshot for a task.
func (s *Store) SaveMetrics(ctx context.Context, rec MetricsRecord) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO metrics(task_id, total_duration_ms, sandbox_duration_ms, tool_call_count, permission_block_count, finding_count, severity_counts_json, exception_counts_json, redaction_count, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(task_id) DO UPDATE SET
total_duration_ms=excluded.total_duration_ms,
sandbox_duration_ms=excluded.sandbox_duration_ms,
tool_call_count=excluded.tool_call_count,
permission_block_count=excluded.permission_block_count,
finding_count=excluded.finding_count,
severity_counts_json=excluded.severity_counts_json,
exception_counts_json=excluded.exception_counts_json,
redaction_count=excluded.redaction_count,
created_at=excluded.created_at
`, rec.TaskID, rec.TotalDurationMS, rec.SandboxDurationMS, rec.ToolCallCount, rec.PermissionBlockCount, rec.FindingCount, rec.SeverityCountsJSON, rec.ExceptionCountsJSON, rec.RedactionCount, rec.At.UTC().Format(time.RFC3339Nano))
	return err
}

// MetricsByTaskID loads the telemetry snapshot for one task.
func (s *Store) MetricsByTaskID(ctx context.Context, taskID string) (MetricsSummary, error) {
	var out MetricsSummary
	var createdAt string
	err := s.db.QueryRowContext(ctx, `
SELECT task_id, total_duration_ms, sandbox_duration_ms, tool_call_count, permission_block_count, finding_count, severity_counts_json, exception_counts_json, redaction_count, created_at
FROM metrics WHERE task_id=?
`, taskID).Scan(&out.TaskID, &out.TotalDurationMS, &out.SandboxDurationMS, &out.ToolCallCount, &out.PermissionBlockCount, &out.FindingCount, &out.SeverityCountsJSON, &out.ExceptionCountsJSON, &out.RedactionCount, &createdAt)
	if err != nil {
		return MetricsSummary{}, err
	}
	out.At, _ = time.Parse(time.RFC3339Nano, createdAt)
	return out, nil
}

// nullableTime converts an optional time to a database-friendly value.
func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// parseNullableTime turns a nullable text timestamp back into time.Time.
func parseNullableTime(v sql.NullString) time.Time {
	if !v.Valid || v.String == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, v.String)
	return t
}
