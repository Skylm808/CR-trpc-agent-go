// Package input turns supported review inputs into unified diff bytes and
// extracts minimal Go project metadata from that diff.
package input

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
)

// Config holds input loader settings.
type Config struct {
	// FixturesRoot is the controlled fixture directory.
	FixturesRoot string
}

// Request describes one review input source.
type Request struct {
	// DiffFile is an external unified diff.
	DiffFile string
	// FileList is a newline-delimited changed file list.
	FileList string
	// RepoPath is a local Git workspace or plain directory.
	RepoPath string
	// Fixture is a fixture name under Config.FixturesRoot.
	Fixture string
	// BaseRef is the base git ref for repo diffs.
	BaseRef string
	// HeadRef is the head git ref for repo diffs.
	HeadRef string
}

// Read reads or generates unified diff input.
func Read(cfg Config, req Request) ([]byte, string, error) {
	if req.DiffFile != "" {
		b, err := os.ReadFile(req.DiffFile)
		return b, req.DiffFile, err
	}
	if req.FileList != "" {
		b, err := diffFromFileList(req.FileList, req.RepoPath)
		return b, req.FileList, err
	}
	if req.Fixture != "" {
		return readFixtureInput(cfg.FixturesRoot, req.Fixture)
	}
	if req.RepoPath != "" {
		b, err := diffFromRepo(req.RepoPath, req.BaseRef, req.HeadRef)
		return b, req.RepoPath, err
	}
	return nil, "", errors.New("diff file, file list, repo path, or fixture is required")
}

func readFixtureInput(root string, name string) ([]byte, string, error) {
	if strings.TrimSpace(root) == "" {
		return nil, "", errors.New("fixtures root is required")
	}
	cleanName := filepath.Clean(strings.TrimSpace(name))
	if cleanName == "." || filepath.IsAbs(cleanName) || strings.HasPrefix(cleanName, "..") {
		return nil, "", fmt.Errorf("invalid fixture name %q", name)
	}
	path := filepath.Join(root, cleanName)
	b, err := os.ReadFile(path)
	return b, path, err
}

func diffFromRepo(repoPath string, baseRef string, headRef string) ([]byte, error) {
	if repoPath == "" {
		return nil, errors.New("repo path is required")
	}
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
		args := []string{"-C", repoPath, "diff", "--unified=3"}
		if strings.TrimSpace(baseRef) != "" && strings.TrimSpace(headRef) != "" {
			args = append(args, strings.TrimSpace(baseRef)+"..."+strings.TrimSpace(headRef))
		}
		cmd := exec.Command("git", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("git diff: %w: %s", err, string(out))
		}
		return out, nil
	}
	var b strings.Builder
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(repoPath, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		lines := strings.Split(string(content), "\n")
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n", entry.Name(), entry.Name())
		fmt.Fprintf(&b, "--- /dev/null\n+++ b/%s\n", entry.Name())
		fmt.Fprintf(&b, "@@ -0,0 +1,%d @@\n", len(lines))
		for _, line := range lines {
			if line == "" {
				continue
			}
			fmt.Fprintf(&b, "+%s\n", line)
		}
	}
	return []byte(b.String()), nil
}

func diffFromFileList(listPath string, repoPath string) ([]byte, error) {
	content, err := os.ReadFile(listPath)
	if err != nil {
		return nil, err
	}
	baseDir := filepath.Dir(listPath)
	restrictToBase := false
	if strings.TrimSpace(repoPath) != "" {
		baseDir = repoPath
		restrictToBase = true
	}
	var b strings.Builder
	for _, raw := range strings.Split(string(content), "\n") {
		name := strings.TrimSpace(raw)
		if name == "" || strings.HasPrefix(name, "#") {
			continue
		}
		path, display, err := resolveListedFile(name, baseDir, restrictToBase)
		if err != nil {
			return nil, err
		}
		fileContent, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read listed file %q: %w", name, err)
		}
		diffForNewFile(&b, display, fileContent)
	}
	return []byte(b.String()), nil
}

func resolveListedFile(name string, baseDir string, restrictToBase bool) (string, string, error) {
	path := filepath.Clean(name)
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	display := filepath.Base(path)
	if rel, err := filepath.Rel(baseDir, path); err == nil {
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			if restrictToBase || !filepath.IsAbs(name) {
				return "", "", fmt.Errorf("listed file %q escapes base directory", name)
			}
		} else {
			display = rel
		}
	}
	return path, display, nil
}

func diffForNewFile(b *strings.Builder, name string, content []byte) {
	display := filepath.ToSlash(strings.TrimPrefix(filepath.Clean(name), string(filepath.Separator)))
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	fmt.Fprintf(b, "diff --git a/%s b/%s\n", display, display)
	fmt.Fprintf(b, "--- /dev/null\n+++ b/%s\n", display)
	fmt.Fprintf(b, "@@ -0,0 +1,%d @@\n", nonEmptyLineCount(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		fmt.Fprintf(b, "+%s\n", line)
	}
}

func nonEmptyLineCount(lines []string) int {
	count := 0
	for _, line := range lines {
		if line != "" {
			count++
		}
	}
	return count
}

// Metadata extracts minimal Go project metadata from diff input.
func Metadata(diff []byte, repoPath string) review.InputMetadata {
	parsed, err := review.ParseUnifiedDiff(string(diff))
	if err != nil {
		return review.InputMetadata{ModulePath: modulePath(repoPath)}
	}
	goFiles := map[string]struct{}{}
	packages := map[string]struct{}{}
	testFiles := map[string]struct{}{}
	for _, file := range parsed.Files {
		if !strings.HasSuffix(file.Path, ".go") {
			continue
		}
		goFiles[file.Path] = struct{}{}
		if strings.HasSuffix(file.Path, "_test.go") {
			testFiles[file.Path] = struct{}{}
		}
		for _, hunk := range file.Hunks {
			for _, line := range hunk.Lines {
				if pkg := packageNameFromLine(line.Text); pkg != "" {
					packages[pkg] = struct{}{}
				}
			}
		}
	}
	return review.InputMetadata{
		ChangedGoFiles:   sortedKeys(goFiles),
		PackageNames:     sortedKeys(packages),
		ModulePath:       modulePath(repoPath),
		HasTests:         len(testFiles) > 0,
		TouchedTestFiles: sortedKeys(testFiles),
	}
}

// MetadataForRequest extracts metadata and includes request git refs.
func MetadataForRequest(diff []byte, req Request) review.InputMetadata {
	meta := Metadata(diff, req.RepoPath)
	meta.BaseRef = strings.TrimSpace(req.BaseRef)
	meta.HeadRef = strings.TrimSpace(req.HeadRef)
	return meta
}

func packageNameFromLine(line string) string {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) >= 2 && fields[0] == "package" {
		return fields[1]
	}
	return ""
}

func modulePath(repoPath string) string {
	if strings.TrimSpace(repoPath) == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(repoPath, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1]
		}
	}
	return ""
}

func sortedKeys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, filepath.ToSlash(value))
	}
	sort.Strings(out)
	return out
}
