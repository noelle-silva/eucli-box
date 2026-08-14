package releaseverify

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"devtools/common/releaseartifact"
	"devtools/common/releaseasset"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
	"eucli-box/pkg/workspace"
)

// buildConcurrency 是成品制作并发上限：机器同时制作多个成品，避免 CPU 与磁盘争抢反而变慢。
const buildConcurrency = 4

// artifactResult 是单个成品制作的并发结果；按目录顺序收集后统一记录。
type artifactResult struct {
	target   string
	ok       bool
	manifest types.ReleaseManifest
	err      error
	elapsed  time.Duration
}

func VerifyReleaseBuild(ctx context.Context, repositoryRoot string, runRoot string) error {
	paths, err := prepareRun(repositoryRoot, runRoot, "verify-release-build")
	if err != nil {
		return err
	}
	recorder := newRecorder("verify-release-build", "full", paths.root)
	fmt.Printf("发布成品制作验证目录：%s\n", paths.root)

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
		artifacts := catalog.SortedArtifacts()
		// 先串行预取全部外部附带内容，再并发制作，避免并发时资产准备互相竞争。
		prefetchAssets(ctx, repositoryRoot, artifacts, paths)
		buildArtifactsConcurrently(ctx, repositoryRoot, artifacts, paths, recorder)
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

// prefetchAssets 串行预取全部发布物需要的外部附带内容。
func prefetchAssets(ctx context.Context, root string, artifacts []types.ReleaseArtifactIdentity, paths runPaths) {
	assetRoot := workspace.VerificationAssetRoot(root)
	for _, identity := range artifacts {
		started := time.Now()
		target := releasecatalog.Target(identity)
		_, err := releaseasset.PrepareRequired(ctx, releaseasset.PrepareOptions{
			RepositoryRoot: root,
			Artifact:       identity,
			OutputRoot:     filepath.Join(assetRoot, "prepared"),
			CacheRoot:      filepath.Join(assetRoot, "cache"),
			TempRoot:       filepath.Join(assetRoot, "temp"),
		})
		if err != nil {
			fmt.Printf("[验证] 失败：外部内容预取 %s（%s）\n", target, formatElapsed(time.Since(started)))
		} else {
			fmt.Printf("[验证] 完成：外部内容预取 %s（%s）\n", target, formatElapsed(time.Since(started)))
		}
	}
}

// buildArtifactsConcurrently 按固定并发上限排队制作全部成品，结果按目录顺序统一记录。
func buildArtifactsConcurrently(ctx context.Context, root string, artifacts []types.ReleaseArtifactIdentity, paths runPaths, recorder *recorder) {
	results := make([]artifactResult, len(artifacts))
	semaphore := make(chan struct{}, buildConcurrency)
	var waitGroup sync.WaitGroup
	for index, identity := range artifacts {
		waitGroup.Add(1)
		go func(index int, identity types.ReleaseArtifactIdentity) {
			defer waitGroup.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			target := releasecatalog.Target(identity)
			started := time.Now()
			fmt.Printf("[验证] 开始：制作并验收 %s（第 %d/%d 个）\n", target, index+1, len(artifacts))
			evidenceRoot := filepath.Join(paths.evidence, "artifacts", strings.ReplaceAll(target, ":", "-"))
			result, buildErr := releaseartifact.Build(ctx, releaseartifact.BuildOptions{
				Root:             root,
				Target:           target,
				WorkRoot:         filepath.Join(paths.workspace, "build"),
				OutputRoot:       filepath.Join(paths.workspace, "output"),
				EvidenceRoot:     evidenceRoot,
				VerificationOnly: true,
				AssetRoot:        workspace.VerificationAssetRoot(root),
			})
			elapsed := time.Since(started)
			if buildErr != nil {
				results[index] = artifactResult{target: target, err: buildErr, elapsed: elapsed}
				fmt.Printf("[验证] 失败：制作并验收 %s（%s，%s）\n", target, buildErr.Error(), formatElapsed(elapsed))
				return
			}
			if result.Manifest.Artifact != identity || !result.Manifest.VerificationOnly || result.Manifest.Platform != "windows-x64" {
				results[index] = artifactResult{target: target, err: fmt.Errorf("成品身份、平台或验证标记不一致"), elapsed: elapsed}
				fmt.Printf("[验证] 失败：制作并验收 %s（身份、平台或验证标记不一致，%s）\n", target, formatElapsed(elapsed))
				return
			}
			results[index] = artifactResult{target: target, ok: true, manifest: result.Manifest, elapsed: elapsed}
			fmt.Printf("[验证] 完成：制作并验收 %s（%s，%s）\n", target, result.Manifest.Archive.Name, formatElapsed(elapsed))
		}(index, identity)
	}
	waitGroup.Wait()
	for _, result := range results {
		if !result.ok {
			recorder.fail("制作并验收 "+result.target, result.err)
			continue
		}
		recorder.pass("制作并验收 "+result.target, fmt.Sprintf("%s，%d 字节，SHA-256 %s", result.manifest.Archive.Name, result.manifest.Archive.Size, result.manifest.Archive.SHA256))
	}
}
