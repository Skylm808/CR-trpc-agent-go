package agent

import (
	"github.com/Skylm808/CR-trpc-agent-go/internal/input"
	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
)

func readInput(cfg Config, req Request) ([]byte, string, error) {
	return input.Read(inputConfig(cfg), inputRequest(req))
}

func inputMetadata(diff []byte, repoPath string) review.InputMetadata {
	return input.Metadata(diff, repoPath)
}

func inputMetadataForRequest(diff []byte, req Request) review.InputMetadata {
	return input.MetadataForRequest(diff, inputRequest(req))
}

func inputConfig(cfg Config) input.Config {
	return input.Config{FixturesRoot: cfg.FixturesRoot}
}

func inputRequest(req Request) input.Request {
	return input.Request{
		DiffFile: req.DiffFile,
		FileList: req.FileList,
		RepoPath: req.RepoPath,
		Fixture:  req.Fixture,
		BaseRef:  req.BaseRef,
		HeadRef:  req.HeadRef,
	}
}
