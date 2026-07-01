package agent

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// readInput 读取或生成 diff。
func readInput(cfg Config, req Request) ([]byte, string, error) {
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
		b, err := diffFromRepo(req.RepoPath)
		return b, req.RepoPath, err
	}
	return nil, "", errors.New("diff file, file list, repo path, or fixture is required")
}

// readFixtureInput 读取受控样本。
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

// diffFromRepo 从工作区生成 diff。
func diffFromRepo(repoPath string) ([]byte, error) {
	if repoPath == "" {
		return nil, errors.New("repo path is required")
	}
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
		// Git 工作区直接使用 unified diff。
		cmd := exec.Command("git", "-C", repoPath, "diff", "--unified=3")
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
		// 普通目录转换为新增文件 diff。
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

// diffFromFileList 把路径列表转换为新增文件 diff。
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
