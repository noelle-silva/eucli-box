package releaseverify

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"eucli-box/internal/releaseartifact"
	"eucli-box/pkg/releasecatalog"
)

func Stage01(ctx context.Context, repositoryRoot string, runRoot string) error {
	paths, err := prepareRun(repositoryRoot, runRoot, "01")
	if err != nil {
		return err
	}
	recorder := newRecorder("01", "full", paths.root)
	fmt.Printf("阶段一验证目录：%s\n", paths.root)

	dataBefore, dataErr := directorySnapshot(filepath.Join(repositoryRoot, "data"))
	if dataErr != nil {
		recorder.fail("记录真实数据初始状态", dataErr)
	} else {
		recorder.pass("记录真实数据初始状态", "已建立只读完整性快照")
	}
	gitBefore, gitErr := gitSnapshot(repositoryRoot)
	if gitErr != nil {
		recorder.fail("记录源码初始状态", gitErr)
	} else {
		recorder.pass("记录源码初始状态", "已记录当前工作区状态")
	}

	catalog, catalogErr := releasecatalog.Load()
	if catalogErr != nil {
		recorder.fail("读取正式发布物清单", catalogErr)
	} else {
		recorder.pass("读取正式发布物清单", fmt.Sprintf("共 %d 个 Windows x64 独立发布物", len(catalog.Artifacts)))
		for _, identity := range catalog.SortedArtifacts() {
			target := releasecatalog.Target(identity)
			evidenceRoot := filepath.Join(paths.evidence, "artifacts", strings.ReplaceAll(target, ":", "-"))
			result, buildErr := releaseartifact.Build(ctx, releaseartifact.BuildOptions{
				Root:             repositoryRoot,
				Target:           target,
				WorkRoot:         filepath.Join(paths.workspace, "build"),
				OutputRoot:       filepath.Join(paths.workspace, "output"),
				EvidenceRoot:     evidenceRoot,
				VerificationOnly: true,
				AssetCacheRoot:   filepath.Join(paths.cache, "release-assets"),
			})
			if buildErr != nil {
				recorder.fail("制作并验收 "+target, buildErr)
				continue
			}
			if result.Manifest.Artifact != identity || !result.Manifest.VerificationOnly || result.Manifest.Platform != "windows-x64" {
				recorder.fail("制作并验收 "+target, fmt.Errorf("成品身份、平台或验证标记不一致"))
				continue
			}
			recorder.pass("制作并验收 "+target, fmt.Sprintf("%s，%d 字节，SHA-256 %s", result.Manifest.Archive.Name, result.Manifest.Archive.Size, result.Manifest.Archive.SHA256))
		}
	}

	if dataErr == nil {
		dataAfter, snapshotErr := directorySnapshot(filepath.Join(repositoryRoot, "data"))
		if snapshotErr != nil {
			recorder.fail("确认真实数据未改变", snapshotErr)
		} else if err := compareSnapshots("真实 data 目录", dataBefore, dataAfter); err != nil {
			recorder.fail("确认真实数据未改变", err)
		} else {
			recorder.pass("确认真实数据未改变", "制作与启动验收未写入真实 data 目录")
		}
	}
	if gitErr == nil {
		gitAfter, snapshotErr := gitSnapshot(repositoryRoot)
		if snapshotErr != nil {
			recorder.fail("确认源码未被验证改写", snapshotErr)
		} else if err := compareSnapshots("源码工作区", gitBefore, gitAfter); err != nil {
			recorder.fail("确认源码未被验证改写", err)
		} else {
			recorder.pass("确认源码未被验证改写", "验证只在本次隔离目录产生运行内容")
		}
	}
	return recorder.finish(paths)
}
