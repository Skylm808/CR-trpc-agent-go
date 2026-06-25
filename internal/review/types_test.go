package review

import "testing"

func TestFindingKeyDedupesSameFileLineRule(t *testing.T) {
	f1 := Finding{File: "main.go", Line: 12, Category: "resource", RuleID: "close-file"}
	f2 := Finding{File: "main.go", Line: 12, Category: "resource", RuleID: "close-file"}

	if f1.DedupeKey() != f2.DedupeKey() {
		t.Fatalf("expected identical dedupe keys, got %q and %q", f1.DedupeKey(), f2.DedupeKey())
	}
}

func TestRedactSecretsMasksCommonTokenShapes(t *testing.T) {
	got := RedactSecrets("apiKey=sk-1234567890abcdef password=hello token=abc.def.ghi")
	if got == "apiKey=sk-1234567890abcdef password=hello token=abc.def.ghi" {
		t.Fatal("expected secrets to be redacted")
	}
}

