package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"eucli-box/internal/releaseartifact"
	"eucli-box/internal/releasepublish"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
	"eucli-box/pkg/workspace"
)

type publishReport struct {
	Build     releaseartifact.BuildResult `json:"build"`
	Publish   releasepublish.Result       `json:"publish"`
	IndexPush bool                        `json:"indexPush"`
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
	catalog, identity, err := resolveTarget(*target)
	if err != nil {
		return err
	}
	source, err := catalog.SourceFor(identity.Kind)
	if err != nil {
		return err
	}
	token, err := githubToken(root, identity.Kind)
	if err != nil {
		return fmt.Errorf("读取正式发布凭据失败：%w", err)
	}
	workParent := workspace.WorkRoot(root)
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
		AssetRoot:        workspace.AssetRoot(root),
	})
	if err != nil {
		return fmt.Errorf("正式发布制作失败，现场保留在 %s：%w", runRoot, err)
	}
	publisher, err := releasepublish.New(releasepublish.Config{Token: token})
	if err != nil {
		return fmt.Errorf("正式发布准备失败，现场保留在 %s：%w", runRoot, err)
	}
	publishResult, err := publisher.Publish(ctx, releasepublish.PublishInput{
		ArchivePath:  buildResult.ArchivePath,
		ManifestPath: buildResult.ManifestPath,
		NotesPath:    buildResult.NotesPath,
	})
	if err != nil {
		return fmt.Errorf("正式发布失败，现场保留在 %s：%w", runRoot, err)
	}
	notes, err := os.ReadFile(buildResult.NotesPath)
	if err != nil {
		return fmt.Errorf("正式发布已公开但读取发行说明失败，现场保留在 %s：%w", runRoot, err)
	}
	indexUpdate := releasepublish.IndexUpdate{
		Artifact:       identity,
		Version:        buildResult.Manifest.Version,
		PublishedAt:    time.Now().UTC(),
		SourceRevision: buildResult.Manifest.Source.Commit,
		DataVersion:    buildResult.Manifest.DataVersion,
		Compatibility:  buildResult.Manifest.Compatibility,
		ReleaseNotes:   string(notes),
		Package: releasecatalog.IndexPackage{
			Platform:   types.ReleasePlatformWindowsX64,
			ReleaseTag: buildResult.Manifest.TagName,
			FileName:   buildResult.Manifest.Archive.Name,
			SizeBytes:  buildResult.Manifest.Archive.Size,
			SHA256:     buildResult.Manifest.Archive.SHA256,
		},
	}
	if err := publisher.UpdateIndex(ctx, source, indexUpdate); err != nil {
		return fmt.Errorf("正式发布已公开但索引登记失败，现场保留在 %s：%w", runRoot, err)
	}
	finalRoot := workspace.OutputRoot(root)
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
	report := publishReport{Build: buildResult, Publish: publishResult, IndexPush: true}
	logPath := filepath.Join(workspace.LogsRoot(root), runLabel("publish")+".json")
	if err := writeJSONFile(logPath, report); err != nil {
		return fmt.Errorf("远端已经公开且本地成品已保存，但写入脱敏记录失败：%w", err)
	}
	completed = true
	return printJSON(report)
}
