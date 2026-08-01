package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/internal/releaseartifact"
	"eucli-box/internal/releasepublish"
)

type publishReport struct {
	Build   releaseartifact.BuildResult `json:"build"`
	Publish releasepublish.Result       `json:"publish"`
}

func runPublish(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("eucli-release publish", flag.ContinueOnError)
	rootValue := flags.String("root", ".", "repository root")
	target := flags.String("target", "", "release target")
	confirmed := flags.Bool("confirm-publish", false, "explicitly allow a real GitHub release")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*target) == "" {
		return fmt.Errorf("必须指定 -target")
	}
	if !*confirmed {
		return fmt.Errorf("正式发布必须显式提供 -confirm-publish")
	}
	root, err := repositoryRoot(*rootValue)
	if err != nil {
		return err
	}
	_, identity, err := resolveTarget(*target)
	if err != nil {
		return err
	}
	token, err := githubToken(root, identity.Kind)
	if err != nil {
		return fmt.Errorf("读取正式发布凭据失败：%w", err)
	}
	releaseRoot := filepath.Join(root, ".release")
	workParent := filepath.Join(releaseRoot, "work")
	if err := os.MkdirAll(workParent, 0o755); err != nil {
		return err
	}
	runRoot, err := os.MkdirTemp(workParent, "publish-")
	if err != nil {
		return err
	}
	completed := false
	defer func() {
		if completed {
			_ = os.RemoveAll(runRoot)
		}
	}()
	buildResult, err := releaseartifact.Build(ctx, releaseartifact.BuildOptions{
		Root:             root,
		Target:           *target,
		WorkRoot:         filepath.Join(runRoot, "build"),
		OutputRoot:       filepath.Join(runRoot, "output"),
		EvidenceRoot:     filepath.Join(runRoot, "evidence"),
		VerificationOnly: false,
		AssetCacheRoot:   filepath.Join(workParent, "asset-cache"),
	})
	if err != nil {
		return fmt.Errorf("正式发布制作失败，现场保留在 %s：%w", runRoot, err)
	}
	publisher, err := releasepublish.New(releasepublish.Config{Token: token})
	if err != nil {
		return fmt.Errorf("正式发布准备失败，现场保留在 %s：%w", runRoot, err)
	}
	publishResult, err := publisher.Publish(ctx, releasepublish.PublishInput{
		Manifest:     buildResult.Manifest,
		ArchivePath:  buildResult.ArchivePath,
		ManifestPath: buildResult.ManifestPath,
		NotesPath:    buildResult.NotesPath,
	})
	if err != nil {
		return fmt.Errorf("正式发布失败，现场保留在 %s：%w", runRoot, err)
	}
	finalRoot := filepath.Join(releaseRoot, "output")
	finalDir := filepath.Join(finalRoot, releaseOutputName(identity), buildResult.Manifest.Version)
	if _, err := os.Stat(finalDir); err == nil {
		return fmt.Errorf("远端已经公开，但本地正式成品目录已存在，现场保留在 %s：%s", runRoot, finalDir)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(finalDir), 0o755); err != nil {
		return err
	}
	if err := os.Rename(buildResult.OutputDir, finalDir); err != nil {
		return fmt.Errorf("远端已经公开，但保存本地正式成品失败，现场保留在 %s：%w", runRoot, err)
	}
	buildResult.OutputDir = finalDir
	buildResult.ArchivePath = filepath.Join(finalDir, filepath.Base(buildResult.ArchivePath))
	buildResult.ManifestPath = filepath.Join(finalDir, filepath.Base(buildResult.ManifestPath))
	buildResult.NotesPath = filepath.Join(finalDir, filepath.Base(buildResult.NotesPath))
	report := publishReport{Build: buildResult, Publish: publishResult}
	logPath := filepath.Join(releaseRoot, "logs", runLabel("publish")+".json")
	if err := writeJSONFile(logPath, report); err != nil {
		return fmt.Errorf("远端已经公开且本地成品已保存，但写入脱敏记录失败：%w", err)
	}
	completed = true
	return printJSON(report)
}
