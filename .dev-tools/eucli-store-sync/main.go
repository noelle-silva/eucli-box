// eucli-store-sync 是本地商店的一键铺货入口：查询开发货架里该发布物当前最大开发尾号，
// 取尾号加 1 生成新开发版本（未显式指定时），构建成品并直接以货架形态就地入库。
// 开发货架根位于工具运行区输出区：output/ai-tools（工具）与 output/system-plugins（插件），
// 业务端直接注册这两个根即可；构建完成即上架，不再有复制步骤。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
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
}

func run(ctx context.Context, args []string) error {
	var opts options
	flags := flag.NewFlagSet("eucli-store-sync", flag.ContinueOnError)
	flags.StringVar(&opts.target, "target", "", "artifact target (tool:<id> or plugin:<id>)")
	flags.StringVar(&opts.version, "version", "", "explicit development version (three or four segments)")
	flags.StringVar(&opts.repoRoot, "repo-root", "", "repository root")
	flags.StringVar(&opts.workRoot, "work-root", "", "build work root")
	flags.StringVar(&opts.outputRoot, "output-root", "", "build output root")
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
	if err := toolruntime.ValidateWorkLocation(root, opts.workRoot, opts.outputRoot); err != nil {
		return err
	}
	workRoot, outputRoot, evidenceRoot, err := resolveRoots(root, opts)
	if err != nil {
		return err
	}
	shelfRoot, err := shelfRootFor(outputRoot, identity.Kind)
	if err != nil {
		return err
	}
	productRoot := filepath.Join(shelfRoot, identity.ID)
	buildVersion, err := resolveBuildVersion(productRoot, artifact.Version, opts.version)
	if err != nil {
		return err
	}
	fmt.Printf("eucli-store-sync: %s -> %s\n", opts.target, buildVersion)
	result, err := releaseartifact.Build(ctx, releaseartifact.BuildOptions{
		Root:            root,
		Target:          string(releaseops.Kind(identity.Kind)) + ":" + identity.ID,
		WorkRoot:        workRoot,
		OutputRoot:      shelfRoot,
		EvidenceRoot:    evidenceRoot,
		ShelfLayout:     true,
		VersionOverride: buildVersion,
	})
	if err != nil {
		return fmt.Errorf("构建失败：%w", err)
	}
	if err := toolruntime.WriteScorecard(toolruntime.Root(root, "eucli-store-sync"), "build", result); err != nil {
		return fmt.Errorf("写入本轮成绩单失败：%w", err)
	}
	fmt.Printf("eucli-store-sync: built %s\n", result.Manifest.Archive.Name)
	fmt.Printf("eucli-store-sync: 已上架 %s\n", shelfRoot)
	return nil
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

// shelfRootFor 返回该类别的开发货架根：构建输出即货架，成品直接落在货架根里。
func shelfRootFor(outputRoot string, kind string) (string, error) {
	switch strings.TrimSpace(kind) {
	case types.ReleaseArtifactKindTool:
		return filepath.Join(outputRoot, "ai-tools"), nil
	case types.ReleaseArtifactKindPlugin:
		return filepath.Join(outputRoot, "system-plugins"), nil
	default:
		return "", fmt.Errorf("本地商店不支持发布物类别 %q（本体候选不上架）", kind)
	}
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
// 否则以该发布物在货架里的历史成品为单一事实源，取同基线最大开发尾号的下一号。
func resolveBuildVersion(productRoot string, sourceVersion string, explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		if err := release.ValidateVersion(strings.TrimSpace(explicit)); err != nil {
			return "", fmt.Errorf("指定版本无效：%w", err)
		}
		return strings.TrimSpace(explicit), nil
	}
	return releaseartifact.NextDevelopmentVersion(productRoot, sourceVersion)
}
