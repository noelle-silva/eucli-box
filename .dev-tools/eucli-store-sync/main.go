// eucli-store-sync 是本地商店的一键铺货入口：查询构建输出区当前最大开发尾号，
// 取尾号加 1 生成新开发版本（未显式指定时），构建成品并复制入架。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"devtools/common/releaseartifact"
	"devtools/common/releaseops"
	"devtools/common/toolruntime"
	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type options struct {
	target     string
	version    string
	repoRoot   string
	workRoot   string
	outputRoot string
	localStore string
	skipCopy   bool
}

func run(ctx context.Context, args []string) error {
	var opts options
	flags := flag.NewFlagSet("eucli-store-sync", flag.ContinueOnError)
	flags.StringVar(&opts.target, "target", "", "artifact target (tool:<id> or plugin:<id>)")
	flags.StringVar(&opts.version, "version", "", "explicit development version (three or four segments)")
	flags.StringVar(&opts.repoRoot, "repo-root", "", "repository root")
	flags.StringVar(&opts.workRoot, "work-root", "", "build work root")
	flags.StringVar(&opts.outputRoot, "output-root", "", "build output root")
	flags.StringVar(&opts.localStore, "local-store", "", "local-store shelf root")
	flags.BoolVar(&opts.skipCopy, "build-only", false, "only build, do not copy to the shelf")
	if err := flags.Parse(args); err != nil {
		return err
	}
	root, err := toolruntime.ValidateRepositoryRoot(opts.repoRoot)
	if err != nil {
		return err
	}
	identity, err := parseTarget(opts.target)
	if err != nil {
		return err
	}
	artifact, err := releaseops.Resolve(root, string(releaseops.Kind(identity.Kind))+":"+identity.ID)
	if err != nil {
		return err
	}
	storeRoot, err := shelfRoot(root, opts.localStore)
	if err != nil {
		return err
	}
	if err := toolruntime.ValidateWorkLocation(root, opts.workRoot, opts.outputRoot); err != nil {
		return err
	}
	workRoot, outputRoot, evidenceRoot, err := resolveRoots(root, opts)
	if err != nil {
		return err
	}
	buildVersion, err := resolveBuildVersion(outputRoot, identity, artifact.Version, opts.version)
	if err != nil {
		return err
	}
	fmt.Printf("eucli-store-sync: %s -> %s\n", opts.target, buildVersion)
	result, err := releaseartifact.Build(ctx, releaseartifact.BuildOptions{
		Root:            root,
		Target:          string(releaseops.Kind(identity.Kind)) + ":" + identity.ID,
		WorkRoot:        workRoot,
		OutputRoot:      outputRoot,
		EvidenceRoot:    evidenceRoot,
		VersionOverride: buildVersion,
	})
	if err != nil {
		return fmt.Errorf("构建失败：%w", err)
	}
	if err := toolruntime.WriteScorecard(toolruntime.Root(root, "eucli-store-sync"), "build", result); err != nil {
		return fmt.Errorf("写入本轮成绩单失败：%w", err)
	}
	fmt.Printf("eucli-store-sync: built %s\n", result.Manifest.Archive.Name)
	if opts.skipCopy {
		return nil
	}
	return runStoreCopy(ctx, root, outputRoot, storeRoot, opts.target)
}

func parseTarget(target string) (types.ReleaseArtifactIdentity, error) {
	kind, id, ok := strings.Cut(strings.TrimSpace(target), ":")
	id = strings.TrimSpace(id)
	if !ok || id == "" || filepath.Base(id) != id {
		return types.ReleaseArtifactIdentity{}, errors.New("必须指定 tool:<id> 或 plugin:<id>")
	}
	kind = strings.TrimSpace(kind)
	switch kind {
	case types.ReleaseArtifactKindTool, types.ReleaseArtifactKindPlugin:
	default:
		return types.ReleaseArtifactIdentity{}, fmt.Errorf("本地商店不支持发布物类别 %q（本体候选不上架）", kind)
	}
	return types.ReleaseArtifactIdentity{Kind: kind, ID: id}, nil
}

func shelfRoot(root string, explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		absolute, err := filepath.Abs(strings.TrimSpace(explicit))
		if err != nil {
			return "", fmt.Errorf("货架根无效：%w", err)
		}
		return absolute, nil
	}
	return filepath.Join(root, ".dev-workspace", ".dev-runtime", "eucli-box", "programs", "local-store"), nil
}

func resolveRoots(root string, opts options) (string, string, string, error) {
	runtimeRoot := toolruntime.Root(root, "eucli-store-sync")
	workRoot := strings.TrimSpace(opts.workRoot)
	if workRoot == "" {
		prepared, err := toolruntime.PrepareRunDir(runtimeRoot, "work", "build")
		if err != nil {
			return "", "", "", fmt.Errorf("建立本轮构建现场失败：%w", err)
		}
		workRoot = prepared
	}
	outputRoot := strings.TrimSpace(opts.outputRoot)
	if outputRoot == "" {
		outputRoot = filepath.Join(runtimeRoot, "output")
	}
	evidenceRoot, err := toolruntime.PrepareRunDir(runtimeRoot, "evidence", "build")
	if err != nil {
		return "", "", "", fmt.Errorf("建立本轮证据现场失败：%w", err)
	}
	return workRoot, outputRoot, evidenceRoot, nil
}

// resolveBuildVersion 决定本次铺货版本：显式指定直接用；
// 否则以构建输出区为单一事实源，在源码正式基线上取同基线最大开发尾号的下一号。
func resolveBuildVersion(outputRoot string, identity types.ReleaseArtifactIdentity, sourceVersion string, explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		if err := release.ValidateVersion(strings.TrimSpace(explicit)); err != nil {
			return "", fmt.Errorf("指定版本无效：%w", err)
		}
		return strings.TrimSpace(explicit), nil
	}
	return releaseartifact.NextDevelopmentVersion(outputRoot, identity, sourceVersion)
}

func runStoreCopy(ctx context.Context, root string, outputRoot string, storeRoot string, target string) error {
	command := exec.CommandContext(ctx, "go", "run", "devtools/eucli-store-copy", "-from", outputRoot, "-to", storeRoot, "-target", target)
	command.Dir = root
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("复制入架失败：%w", err)
	}
	return nil
}
