package review

import (
	"errors"
	"os"
	"path/filepath"
)

func SkillRoot() (string, error) {
	candidates := []string{
		filepath.Join("skills", "code-review"),
		filepath.Join("..", "skills", "code-review"),
		filepath.Join("..", "..", "skills", "code-review"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("code-review skill not found")
}

