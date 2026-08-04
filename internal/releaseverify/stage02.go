package releaseverify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/internal/releaseartifact"
	"eucli-box/internal/releasecredentials"
	"eucli-box/internal/releaseops"
	"eucli-box/internal/releasepublish"
	"eucli-box/pkg/releasecatalog"
)

func Stage02(ctx context.Context, repositoryRoot string, runRoot string, mode string) error {
	paths, err := prepareRun(repositoryRoot, runRoot, "02")
	if err != nil {
		return err
	}
	mode = strings.TrimSpace(mode)
	if mode != "preflight" && mode != "remote" {
		return fmt.Errorf("阶段二模式必须是 preflight 或 remote")
	}
	recorder := newRecorder("02", mode, paths.root)
	fmt.Printf("阶段二 %s 验证目录：%s\n", mode, paths.root)

	dataBefore, dataErr := directorySnapshot(filepath.Join(repositoryRoot, "data"))
	gitBefore, gitErr := gitSnapshot(repositoryRoot)
	if dataErr != nil {
		recorder.fail("记录真实数据初始状态", dataErr)
	}
	if gitErr != nil {
		recorder.fail("记录源码初始状态", gitErr)
	}

	if mode == "preflight" {
		runStage02Preflight(ctx, repositoryRoot, paths, recorder)
	} else {
		runStage02Remote(ctx, repositoryRoot, paths, recorder)
	}

	if dataErr == nil {
		dataAfter, snapshotErr := directorySnapshot(filepath.Join(repositoryRoot, "data"))
		if snapshotErr != nil {
			recorder.fail("确认真实数据未改变", snapshotErr)
		} else if err := compareSnapshots("真实 data 目录", dataBefore, dataAfter); err != nil {
			recorder.fail("确认真实数据未改变", err)
		} else {
			recorder.pass("确认真实数据未改变", "版本检查与远端复核未写入真实数据")
		}
	}
	if gitErr == nil {
		gitAfter, snapshotErr := gitSnapshot(repositoryRoot)
		if snapshotErr != nil {
			recorder.fail("确认源码未被验证改写", snapshotErr)
		} else if err := compareSnapshots("源码工作区", gitBefore, gitAfter); err != nil {
			recorder.fail("确认源码未被验证改写", err)
		} else {
			recorder.pass("确认源码未被验证改写", "阶段二只在本次隔离目录产生运行内容")
		}
	}
	return recorder.finish(paths)
}

func runStage02Preflight(ctx context.Context, root string, paths runPaths, recorder *recorder) {
	commands := []struct {
		name    string
		workdir string
		command string
		args    []string
	}{
		{
			name:    "官方来源、发行名称、重复发布与只读检查",
			workdir: root,
			command: "go",
			args:    []string{"test", "./pkg/releasecatalog", "./internal/releasepublish", "./pkg/releasecheck", "./internal/releaseverify", "-count=1"},
		},
		{
			name:    "业务端自动检查与维护入口",
			workdir: root,
			command: "go",
			args:    []string{"test", "./src/release-check-system", "./src/gateway-system", "-count=1"},
		},
		{
			name:    "客户端独立检查与业务端结果转发",
			workdir: filepath.Join(root, "clients", "eucli-studio", "backend-go"),
			command: "go",
			args:    []string{"test", "./...", "-count=1"},
		},
		{
			name:    "客户端检查结果展示类型",
			workdir: filepath.Join(root, "clients", "eucli-studio"),
			command: "pnpm",
			args:    []string{"exec", "tsc", "--noEmit"},
		},
	}
	for _, command := range commands {
		if err := runCommand(ctx, paths, command.name, command.workdir, command.command, command.args...); err != nil {
			recorder.fail(command.name, err)
		} else {
			recorder.pass(command.name, "隔离测试通过，详细输出见对应 evidence 日志")
		}
	}
}

func runStage02Remote(ctx context.Context, root string, paths runPaths, recorder *recorder) {
	if err := runCommand(ctx, paths, "新版、无新版、来源失败和不下载成品", root, "go", "test", "./pkg/releasecheck", "-count=1"); err != nil {
		recorder.fail("新版、无新版、来源失败和不下载成品", err)
	} else {
		recorder.pass("新版、无新版、来源失败和不下载成品", "隔离官方记录场景全部通过")
	}
	credentials, err := releasecredentials.Load(root)
	if err != nil {
		recorder.fail("读取远端复核凭据", err)
		return
	}
	catalog, err := releasecatalog.Load()
	if err != nil {
		recorder.fail("读取正式发布物清单", err)
		return
	}
	artifacts := catalog.SortedArtifacts()
	for index, identity := range artifacts {
		target := releasecatalog.Target(identity)
		fail := func(reason string, err error) {
			recorder.fail("远端复核 "+target, err)
			fmt.Printf("[验证] 失败：远端复核 %s（%s）\n", target, reason)
		}
		fmt.Printf("[验证] 开始：远端复核 %s（第 %d/%d 个）\n", target, index+1, len(artifacts))
		token, tokenErr := credentials.TokenFor(identity.Kind)
		if tokenErr != nil {
			fail("读取凭据", tokenErr)
			continue
		}
		publisher, publisherErr := releasepublish.New(releasepublish.Config{Token: token})
		if publisherErr != nil {
			fail("准备复核", publisherErr)
			continue
		}
		artifact, resolveErr := releaseops.Resolve(root, target)
		if resolveErr != nil {
			fail("解析版本", resolveErr)
			continue
		}
		remoteRoot := filepath.Join(paths.workspace, "remote", strings.ReplaceAll(target, ":", "-"))
		download, downloadErr := publisher.DownloadPublished(ctx, identity, artifact.Version, filepath.Join(remoteRoot, "download"))
		if downloadErr != nil {
			fail("下载远端成品", downloadErr)
			continue
		}
		verified, verifyErr := releaseartifact.Verify(ctx, releaseartifact.VerifyOptions{
			ArchivePath:  download.ArchivePath,
			ManifestPath: download.ManifestPath,
			Workspace:    filepath.Join(remoteRoot, "verification"),
		})
		if verifyErr != nil {
			fail("验收远端成品", verifyErr)
			continue
		}
		evidenceTarget := filepath.Join(paths.evidence, "remote", strings.ReplaceAll(target, ":", "-")+".json")
		if err := os.MkdirAll(filepath.Dir(evidenceTarget), 0o755); err != nil {
			fail("写入证据", err)
			continue
		}
		summary := map[string]any{
			"artifact":   identity,
			"version":    artifact.Version,
			"releaseUrl": download.ReleaseURL,
			"archive":    verified.Manifest.Archive,
		}
		payload, marshalErr := json.MarshalIndent(summary, "", "  ")
		if marshalErr != nil {
			fail("生成证据", marshalErr)
			continue
		}
		if err := os.WriteFile(evidenceTarget, append(payload, '\n'), 0o644); err != nil {
			fail("写入证据", err)
			continue
		}
		recorder.pass("远端复核 "+target, download.ReleaseURL)
		fmt.Printf("[验证] 完成：远端复核 %s\n", target)
	}
}
