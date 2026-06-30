package review

import (
	"errors"
	"os"
	"path/filepath"
)

// SkillRoot 定位内置 code-review Skill。
func SkillRoot() (string, error) {
	candidates := []string{
		filepath.Join("skills", "code-review"),
		filepath.Join("..", "skills", "code-review"),
		filepath.Join("..", "..", "skills", "code-review"),
	}
	for _, p := range candidates {
		// 兼容包目录和仓库根目录。
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("code-review skill not found")
}
