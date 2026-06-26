package review

import (
	"errors"
	"os"
	"path/filepath"
)

// SkillRoot locates the checked-in code-review skill from common test and CLI
// working directories.
func SkillRoot() (string, error) {
	candidates := []string{
		filepath.Join("skills", "code-review"),
		filepath.Join("..", "skills", "code-review"),
		filepath.Join("..", "..", "skills", "code-review"),
	}
	for _, p := range candidates {
		// Tests may run from the package directory, while the CLI usually runs
		// from the repository root, so try a small set of relative locations.
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("code-review skill not found")
}
