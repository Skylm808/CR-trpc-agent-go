package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAllFixturesGenerateReports(t *testing.T) {
	fixtureRoot := filepath.Join("..", "..", "testdata", "fixtures")
	entries, err := os.ReadDir(fixtureRoot)
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".diff" {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			out := t.TempDir()
			err := Run(Options{
				DiffFile: filepath.Join(fixtureRoot, entry.Name()),
				OutputDir: out,
				Mode: "rule-only",
			})
			if err != nil {
				t.Fatalf("Run returned error: %v", err)
			}
			if _, err := os.Stat(filepath.Join(out, "review_report.json")); err != nil {
				t.Fatalf("expected json report: %v", err)
			}
			if _, err := os.Stat(filepath.Join(out, "review_report.md")); err != nil {
				t.Fatalf("expected markdown report: %v", err)
			}
		})
	}
}

