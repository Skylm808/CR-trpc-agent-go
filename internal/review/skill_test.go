package review

import "testing"

func TestSkillFilesExist(t *testing.T) {
	_, err := SkillRoot()
	if err != nil {
		t.Fatalf("SkillRoot returned error: %v", err)
	}
}

