package review

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSkillFilesExist(t *testing.T) {
	_, err := SkillRoot()
	if err != nil {
		t.Fatalf("SkillRoot returned error: %v", err)
	}
}

// TestSkillCheckScriptFallsBackToGoWhenPythonUnavailable 固定容器兼容性：
// 默认生产容器需要 Go 工具链执行 go test/go vet，Skill 脚本在没有
// python3 时也必须能通过 Go fallback 输出结构化 findings。
func TestSkillCheckScriptFallsBackToGoWhenPythonUnavailable(t *testing.T) {
	t.Parallel()

	skillRoot, err := SkillRoot()
	if err != nil {
		t.Fatalf("SkillRoot returned error: %v", err)
	}
	repoRoot := filepath.Dir(filepath.Dir(skillRoot))
	diff, err := os.ReadFile(filepath.Join(repoRoot, "testdata", "fixtures", "secret.diff"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	bashPath := mustLookPath(t, "bash")
	tempBin := t.TempDir()
	linkTool(t, tempBin, "go")
	linkTool(t, tempBin, "mktemp")
	linkTool(t, tempBin, "cat")
	linkTool(t, tempBin, "rm")

	cmd := exec.Command(bashPath, filepath.Join(skillRoot, "scripts", "check.sh"))
	cmd.Stdin = bytes.NewReader(diff)
	cmd.Env = append(os.Environ(),
		"PATH="+tempBin,
		"GOCACHE="+filepath.Join(t.TempDir(), "gocache"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("check.sh fallback failed: %v\n%s", err, out)
	}

	var payload struct {
		Findings []Finding `json:"findings"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("unmarshal check output: %v\n%s", err, out)
	}
	if len(payload.Findings) != 1 || payload.Findings[0].RuleID != "secret-leak" {
		t.Fatalf("expected secret-leak finding from Go fallback, got %+v", payload.Findings)
	}
}

// mustLookPath 查找测试需要的宿主机工具，缺失时跳过相关兼容性测试。
func mustLookPath(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not available: %v", name, err)
	}
	return path
}

// linkTool 把宿主机工具链接进受控 PATH，用于模拟没有 python3 的容器环境。
func linkTool(t *testing.T, dir string, name string) {
	t.Helper()
	target := mustLookPath(t, name)
	link := filepath.Join(dir, name)
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("link %s: %v", name, err)
	}
}
