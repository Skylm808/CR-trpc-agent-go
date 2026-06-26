package review

import (
	"errors"
	"os"
	"path/filepath"
)

// SkillRoot 从常见的测试和 CLI 工作目录中定位仓库内置的 code-review Skill。
func SkillRoot() (string, error) {
	candidates := []string{
		filepath.Join("skills", "code-review"),
		filepath.Join("..", "skills", "code-review"),
		filepath.Join("..", "..", "skills", "code-review"),
	}
	for _, p := range candidates {
		// 测试可能在包目录下运行，而 CLI 通常在仓库根目录下运行，所以这里尝试几个相对路径。
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("code-review skill not found")
}
