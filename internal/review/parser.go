package review

import (
	"bufio"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var hunkHeader = regexp.MustCompile(`^@@ -(\d+),?(\d*) \+(\d+),?(\d*) @@`)

// ParseUnifiedDiff converts raw diff text into the normalized ParsedDiff
// structure used by the rule engine.
func ParseUnifiedDiff(input string) (ParsedDiff, error) {
	var parsed ParsedDiff
	scanner := bufio.NewScanner(strings.NewReader(input))

	var current *ParsedFile
	var currentHunk *Hunk
	oldLine := 0
	newLine := 0

	flushHunk := func() {
		// Preserve the current hunk before we switch files or start a new one.
		if current == nil || currentHunk == nil {
			return
		}
		current.Hunks = append(current.Hunks, *currentHunk)
		currentHunk = nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flushHunk()
			if current != nil {
				parsed.Files = append(parsed.Files, *current)
			}
			current = &ParsedFile{}
		case strings.HasPrefix(line, "--- "):
			if current == nil {
				current = &ParsedFile{}
			}
		case strings.HasPrefix(line, "+++ "):
			if current == nil {
				current = &ParsedFile{}
			}
			path := strings.TrimPrefix(line, "+++ ")
			path = strings.TrimPrefix(path, "b/")
			path = strings.TrimPrefix(path, "a/")
			path = strings.TrimSpace(path)
			if path != "/dev/null" {
				current.Path = filepath.ToSlash(path)
				current.Language = "go"
				current.IsTestFile = strings.HasSuffix(path, "_test.go")
			}
		case strings.HasPrefix(line, "@@ "):
			flushHunk()
			m := hunkHeader.FindStringSubmatch(line)
			if len(m) != 5 {
				return ParsedDiff{}, fmt.Errorf("invalid hunk header: %q", line)
			}
			oldLine, _ = strconv.Atoi(m[1])
			newLine, _ = strconv.Atoi(m[3])
			currentHunk = &Hunk{
				File:     current.Path,
				OldStart: oldLine,
				NewStart: newLine,
			}
		case currentHunk != nil:
			switch {
			case strings.HasPrefix(line, "+"):
				currentHunk.Lines = append(currentHunk.Lines, Line{NewLine: newLine, Kind: "add", Text: strings.TrimPrefix(line, "+")})
				currentHunk.CandidateLines = append(currentHunk.CandidateLines, newLine)
				newLine++
			case strings.HasPrefix(line, "-"):
				currentHunk.Lines = append(currentHunk.Lines, Line{OldLine: oldLine, Kind: "del", Text: strings.TrimPrefix(line, "-")})
				oldLine++
			default:
				currentHunk.Lines = append(currentHunk.Lines, Line{OldLine: oldLine, NewLine: newLine, Kind: "context", Text: line})
				currentHunk.Context = append(currentHunk.Context, line)
				oldLine++
				newLine++
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return ParsedDiff{}, err
	}
	flushHunk()
	if current != nil {
		parsed.Files = append(parsed.Files, *current)
	}
	for i := range parsed.Files {
		if parsed.Files[i].Path == "" {
			continue
		}
		if parsed.Files[i].Language == "" {
			parsed.Files[i].Language = "go"
		}
	}
	return parsed, nil
}
