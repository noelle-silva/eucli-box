package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"eucli-box/internal/releaseartifact"
)

func runBuild(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("eucli-release build", flag.ContinueOnError)
	rootValue := flags.String("root", ".", "repository root")
	target := flags.String("target", "", "release target")
	workRoot := flags.String("work-root", "", "build workspace root")
	outputRoot := flags.String("output-root", "", "artifact output root")
	evidenceRoot := flags.String("evidence-root", "", "verification evidence root")
	assetCacheRoot := flags.String("asset-cache-root", "", "verified external input cache")
	verificationOnly := flags.Bool("verification-only", false, "mark output as verification only")
	resultFile := flags.String("result-file", "", "write result JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*target) == "" {
		return fmt.Errorf("必须指定 -target")
	}
	root, err := repositoryRoot(*rootValue)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*assetCacheRoot) == "" {
		*assetCacheRoot = filepath.Join(root, ".release", "work", "asset-cache")
	}
	result, err := releaseartifact.Build(ctx, releaseartifact.BuildOptions{
		Root:             root,
		Target:           *target,
		WorkRoot:         *workRoot,
		OutputRoot:       *outputRoot,
		EvidenceRoot:     *evidenceRoot,
		VerificationOnly: *verificationOnly,
		AssetCacheRoot:   *assetCacheRoot,
	})
	if err != nil {
		return err
	}
	if path := strings.TrimSpace(*resultFile); path != "" {
		if err := writeJSONFile(path, result); err != nil {
			return err
		}
	}
	return printJSON(result)
}
