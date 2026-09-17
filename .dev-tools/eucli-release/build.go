package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"devtools/common/releaseartifact"
	"devtools/common/releaseops"
	"devtools/common/toolruntime"
	"eucli-box/pkg/workspace"
)

func runBuild(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("eucli-release build", flag.ContinueOnError)
	rootValue := flags.String("root", ".", "repository root")
	target := flags.String("target", "", "release target")
	workRoot := flags.String("work-root", "", "build workspace root")
	outputRoot := flags.String("output-root", "", "artifact output root")
	evidenceRoot := flags.String("evidence-root", "", "verification evidence root")
	assetRoot := flags.String("asset-root", "", "verified external asset root")
	versionOverride := flags.String("version-override", "", "development build version (three or four segments)")
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
	if strings.TrimSpace(*target) == string(releaseops.KindBox) {
		return runPackBox(ctx, root, *workRoot, *outputRoot, *evidenceRoot, *assetRoot, *versionOverride, *resultFile)
	}
	if err := toolruntime.ValidateWorkLocation(root, *workRoot, *outputRoot, *evidenceRoot); err != nil {
		return err
	}
	runtimeRoot := toolruntime.Root(root, "eucli-release")
	resolvedWorkRoot := strings.TrimSpace(*workRoot)
	if resolvedWorkRoot == "" {
		resolvedWorkRoot, err = toolruntime.PrepareRunDir(runtimeRoot, "work", "build")
		if err != nil {
			return err
		}
	}
	resolvedOutputRoot := filepath.Join(runtimeRoot, "output")
	if strings.TrimSpace(*outputRoot) != "" {
		resolvedOutputRoot = strings.TrimSpace(*outputRoot)
	}
	resolvedEvidenceRoot := strings.TrimSpace(*evidenceRoot)
	if resolvedEvidenceRoot == "" {
		resolvedEvidenceRoot, err = toolruntime.PrepareRunDir(runtimeRoot, "evidence", "build")
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(*assetRoot) == "" {
		*assetRoot = workspace.AssetRoot(root)
	}
	result, err := releaseartifact.Build(ctx, releaseartifact.BuildOptions{
		Root:            root,
		Target:          *target,
		WorkRoot:        resolvedWorkRoot,
		OutputRoot:      resolvedOutputRoot,
		EvidenceRoot:    resolvedEvidenceRoot,
		VersionOverride: *versionOverride,
		AssetRoot:       *assetRoot,
	})
	if err != nil {
		return err
	}
	if err := toolruntime.WriteScorecard(runtimeRoot, "build", result); err != nil {
		return err
	}
	if path := strings.TrimSpace(*resultFile); path != "" {
		if err := writeJSONFile(path, result); err != nil {
			return err
		}
	}
	return printJSON(result)
}

// runPackBox 执行业务端本体的简版打包：只构建可分发压缩包，不制作正式成品。
// 正式成品专属参数对本体一概拒绝，避免调用方误以为仍走正式流程。
func runPackBox(ctx context.Context, root string, workRoot string, outputRoot string, evidenceRoot string, assetRoot string, versionOverride string, resultFile string) error {
	for _, rejected := range []struct {
		name  string
		value string
	}{
		{name: "-evidence-root", value: evidenceRoot},
		{name: "-asset-root", value: assetRoot},
		{name: "-version-override", value: versionOverride},
	} {
		if strings.TrimSpace(rejected.value) != "" {
			return fmt.Errorf("本体打包不支持 %s：本体已退出正式成品制作", rejected.name)
		}
	}
	if err := toolruntime.ValidateWorkLocation(root, workRoot, outputRoot); err != nil {
		return err
	}
	runtimeRoot := toolruntime.Root(root, "eucli-release")
	resolvedWorkRoot := strings.TrimSpace(workRoot)
	if resolvedWorkRoot == "" {
		prepared, err := toolruntime.PrepareRunDir(runtimeRoot, "work", "build")
		if err != nil {
			return err
		}
		resolvedWorkRoot = prepared
	}
	resolvedOutputRoot := filepath.Join(runtimeRoot, "output")
	if strings.TrimSpace(outputRoot) != "" {
		resolvedOutputRoot = strings.TrimSpace(outputRoot)
	}
	result, err := releaseartifact.PackBox(ctx, releaseartifact.PackBoxOptions{
		Root:       root,
		WorkRoot:   resolvedWorkRoot,
		OutputRoot: resolvedOutputRoot,
	})
	if err != nil {
		return err
	}
	if err := toolruntime.WriteScorecard(runtimeRoot, "build", result); err != nil {
		return err
	}
	if path := strings.TrimSpace(resultFile); path != "" {
		if err := writeJSONFile(path, result); err != nil {
			return err
		}
	}
	return printJSON(result)
}
