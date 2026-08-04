package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/internal/releaseartifact"
	"eucli-box/internal/releaseops"
	"eucli-box/internal/releasepublish"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/workspace"
)

type remoteReport struct {
	ReleaseURL string                       `json:"releaseUrl"`
	Manifest   any                          `json:"manifest"`
	Verified   releaseartifact.VerifyResult `json:"verified"`
}

func runRemote(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("eucli-release remote", flag.ContinueOnError)
	rootValue := flags.String("root", ".", "repository root")
	target := flags.String("target", "", "release target")
	version := flags.String("version", "", "released version; defaults to local version")
	workspaceValue := flags.String("workspace", "", "remote verification workspace")
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
	_, identity, err := resolveTarget(*target)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*version) == "" {
		artifact, err := releaseops.Resolve(root, releasecatalog.Target(identity))
		if err != nil {
			return err
		}
		*version = artifact.Version
	}
	workRoot := strings.TrimSpace(*workspaceValue)
	if workRoot == "" {
		parent := workspace.WorkRoot(root)
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return err
		}
		workRoot, err = os.MkdirTemp(parent, "remote-")
		if err != nil {
			return err
		}
	} else {
		workRoot, err = filepath.Abs(workRoot)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(workRoot, 0o755); err != nil {
			return err
		}
	}
	token, err := githubToken(root, identity.Kind)
	if err != nil {
		return fmt.Errorf("读取远端复核凭据失败：%w", err)
	}
	publisher, err := releasepublish.New(releasepublish.Config{Token: token})
	if err != nil {
		return err
	}
	download, err := publisher.DownloadPublished(ctx, identity, *version, filepath.Join(workRoot, "download"))
	if err != nil {
		return fmt.Errorf("远端只读复核失败，现场保留在 %s：%w", workRoot, err)
	}
	verified, err := releaseartifact.Verify(ctx, releaseartifact.VerifyOptions{
		ArchivePath:  download.ArchivePath,
		ManifestPath: download.ManifestPath,
		Workspace:    filepath.Join(workRoot, "verification"),
	})
	if err != nil {
		return fmt.Errorf("远端成品验收失败，现场保留在 %s：%w", workRoot, err)
	}
	report := remoteReport{ReleaseURL: download.ReleaseURL, Manifest: download.Manifest, Verified: verified}
	if path := strings.TrimSpace(*resultFile); path != "" {
		if err := writeJSONFile(path, report); err != nil {
			return err
		}
	}
	return printJSON(report)
}
