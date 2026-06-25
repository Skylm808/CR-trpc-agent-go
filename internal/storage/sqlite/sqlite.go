package sqlite

import (
	"context"
	"database/sql"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
)

type Store struct {
	db *sql.DB
}

type Task struct {
	ID         string
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

type Report struct {
	JSON      []byte
	Markdown  []byte
	CreatedAt time.Time
}

type DecisionRecord struct {
	TaskID  string
	Command string
	Action  string
	Reason  string
	At      time.Time
}

type SandboxRunRecord struct {
	TaskID  string
	Command string
	Status  string
	Output  string
	At      time.Time
}

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
CREATE TABLE IF NOT EXISTS sandbox_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id TEXT NOT NULL,
  command TEXT NOT NULL,
  status TEXT NOT NULL,
  output TEXT,
  created_at TEXT NOT NULL
);
`)
	return err
}

func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

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

func (s *Store) SaveFinding(ctx context.Context, taskID string, finding review.Finding) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO findings(finding_id, task_id, severity, category, file, line, title, evidence, recommendation, confidence, source, rule_id, dedupe_key, status)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`,
		finding.DedupeKey(), taskID, finding.Severity, finding.Category, finding.File, finding.Line, finding.Title,
		finding.Evidence, finding.Recommendation, finding.Confidence, finding.Source, finding.RuleID, finding.DedupeKey(), finding.Status)
	return err
}

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

func (s *Store) SaveDecision(ctx context.Context, rec DecisionRecord) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO permission_decisions(task_id, command, action, reason, created_at)
VALUES(?, ?, ?, ?, ?)
`, rec.TaskID, rec.Command, rec.Action, rec.Reason, rec.At.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) SaveSandboxRun(ctx context.Context, rec SandboxRunRecord) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO sandbox_runs(task_id, command, status, output, created_at)
VALUES(?, ?, ?, ?, ?)
`, rec.TaskID, rec.Command, rec.Status, rec.Output, rec.At.UTC().Format(time.RFC3339Nano))
	return err
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseNullableTime(v sql.NullString) time.Time {
	if !v.Valid || v.String == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, v.String)
	return t
}
