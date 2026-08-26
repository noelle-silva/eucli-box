// eucli-store-copy 把构建输出区的成品（zip + 清单）复制到本地商店货架
// programs\local-store\ 的对应货架目录；目标已存在同版本时直接就地覆盖。
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
	from   string
	to     string
	target string
}

func run(ctx context.Context, args []string) error {
	var opts options
	flags := flag.NewFlagSet("eucli-store-copy", flag.ContinueOnError)
	flags.StringVar(&opts.from, "from", "", "build output root (contains <kind>-<id>/<version>/)")
	flags.StringVar(&opts.to, "to", "", "local-store shelf root (created if missing)")
	flags.StringVar(&opts.target, "target", "", "artifact target (tool:<id> or plugin:<id>); empty means all")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(opts.from) == "" {
		return errors.New("必须指定 -from")
	}
	if strings.TrimSpace(opts.to) == "" {
		return errors.New("必须指定 -to")
	}
	from, err := filepath.Abs(strings.TrimSpace(opts.from))
	if err != nil {
		return fmt.Errorf("确定构建输出根失败：%w", err)
	}
	info, err := os.Stat(from)
	if err != nil {
		return fmt.Errorf("读取构建输出根失败：%w", err)
	}
	if !info.IsDir() {
		return errors.New("构建输出根必须是目录")
	}
	to, err := filepath.Abs(strings.TrimSpace(opts.to))
	if err != nil {
		return fmt.Errorf("确定货架根失败：%w", err)
	}
	if err := os.MkdirAll(to, 0o755); err != nil {
		return fmt.Errorf("建立货架根失败：%w", err)
	}
	target := strings.TrimSpace(opts.target)
	if target == "" {
		return copyAll(ctx, from, to)
	}
	return copyTarget(ctx, from, to, target)
}

type shelfPlacement struct {
	productDir string
	shelfDir   string
}

// kindShelfDir 把发布物类别映射到货架子目录，并拒绝不能上架的类别。
func kindShelfDir(kind string) (string, error) {
	switch strings.TrimSpace(kind) {
	case types.ReleaseArtifactKindTool:
		return "ai-tools", nil
	case types.ReleaseArtifactKindPlugin:
		return "system-plugins", nil
	default:
		return "", fmt.Errorf("本地商店不支持发布物类别 %q（本体候选不上架）", kind)
	}
}

func copyAll(ctx context.Context, from string, to string) error {
	entries, err := os.ReadDir(from)
	if err != nil {
		return fmt.Errorf("读取构建输出根失败：%w", err)
	}
	copied := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		identity, ok := parseProductDir(entry.Name())
		if !ok {
			continue
		}
		if err := copyOne(ctx, from, to, identity); err != nil {
			return err
		}
		copied++
	}
	fmt.Printf("eucli-store-copy: %d artifact(s) copied to %s\n", copied, to)
	return nil
}

func copyTarget(ctx context.Context, from string, to string, target string) error {
	identity, err := parseTarget(target)
	if err != nil {
		return err
	}
	product := identity.Kind + "-" + identity.ID
	if _, err := os.Stat(filepath.Join(from, product)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("构建输出没有 %s 的成品", target)
		}
		return err
	}
	if err := copyOne(ctx, from, to, identity); err != nil {
		return err
	}
	fmt.Printf("eucli-store-copy: %s copied to %s\n", target, to)
	return nil
}

// copyOne 把 <kind>-<id>/<version>/ 全部版本证书复制到货架，同版本覆盖。
func copyOne(ctx context.Context, from string, to string, identity types.ReleaseArtifactIdentity) error {
	kindDir, err := kindShelfDir(identity.Kind)
	if err != nil {
		return err
	}
	product := identity.Kind + "-" + identity.ID
	versionsDir := filepath.Join(from, product)
	versionEntries, err := os.ReadDir(versionsDir)
	if err != nil {
		return err
	}
	for _, versionEntry := range versionEntries {
		if !versionEntry.IsDir() {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		version := versionEntry.Name()
		if err := release.ValidateVersion(version); err != nil {
			continue
		}
		assets, err := productAssets(filepath.Join(versionsDir, version))
		if err != nil {
			return fmt.Errorf("%s %s：%w", product, version, err)
		}
		if err := verifyProductAssets(identity, version, assets); err != nil {
			return fmt.Errorf("%s %s：%w", product, version, err)
		}
		dest := filepath.Join(to, kindDir, identity.ID, version)
		if err := os.RemoveAll(dest); err != nil {
			return fmt.Errorf("覆盖旧货品失败：%w", err)
		}
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return fmt.Errorf("建立货品目录失败：%w", err)
		}
		for name, source := range assets {
			if err := copyFile(source, filepath.Join(dest, name)); err != nil {
				return err
			}
		}
		fmt.Printf("  %s/%s/%s ✓\n", kindDir, identity.ID, version)
	}
	return nil
}

type assetPair struct {
	manifest string
	archive  string
}

func productAssets(versionDir string) (map[string]string, error) {
	entries, err := os.ReadDir(versionDir)
	if err != nil {
		return nil, err
	}
	assets := map[string]string{}
	manifestCount := 0
	archiveCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(strings.ToLower(name), ".manifest.json") {
			manifestCount++
			assets[name] = filepath.Join(versionDir, name)
		} else if strings.HasSuffix(strings.ToLower(name), ".zip") {
			archiveCount++
			assets[name] = filepath.Join(versionDir, name)
		}
	}
	if manifestCount != 1 || archiveCount != 1 {
		return nil, fmt.Errorf("版本目录必须恰好包含一个清单与一个压缩包（实际 %d/%d）", manifestCount, archiveCount)
	}
	return assets, nil
}

func verifyProductAssets(identity types.ReleaseArtifactIdentity, version string, assets map[string]string) error {
	var manifestPath, archivePath string
	for name, path := range assets {
		if strings.HasSuffix(strings.ToLower(name), ".manifest.json") {
			manifestPath = path
		} else {
			archivePath = path
		}
	}
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	manifest, err := release.DecodeReleaseManifest(payload)
	if err != nil {
		return err
	}
	if manifest.Artifact != identity || manifest.Version != version {
		return errors.New("清单身份或版本与货位不一致")
	}
	expectedName := filepath.Base(archivePath)
	if manifest.Archive.Name != expectedName {
		return errors.New("清单压缩包名与实际文件不一致")
	}
	size, sha256, err := recordForFile(archivePath)
	if err != nil {
		return err
	}
	if size != manifest.Archive.Size || !strings.EqualFold(sha256, manifest.Archive.SHA256) {
		return errors.New("压缩包摘要与清单不一致")
	}
	return nil
}

func parseTarget(target string) (types.ReleaseArtifactIdentity, error) {
	kind, id, ok := strings.Cut(strings.TrimSpace(target), ":")
	id = strings.TrimSpace(id)
	if !ok || id == "" || filepath.Base(id) != id {
		return types.ReleaseArtifactIdentity{}, fmt.Errorf("目标必须是 tool:<id> 或 plugin:<id>")
	}
	return types.ReleaseArtifactIdentity{Kind: strings.TrimSpace(kind), ID: id}, nil
}

func parseProductDir(name string) (types.ReleaseArtifactIdentity, bool) {
	for _, kind := range []string{types.ReleaseArtifactKindTool, types.ReleaseArtifactKindPlugin} {
		if id, ok := strings.CutPrefix(name, kind+"-"); ok && id != "" && filepath.Base(id) == id {
			return types.ReleaseArtifactIdentity{Kind: kind, ID: id}, true
		}
	}
	return types.ReleaseArtifactIdentity{}, false
}

func recordForFile(path string) (int64, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, "", err
	}
	if !info.Mode().IsRegular() {
		return 0, "", errors.New("不是普通文件")
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer file.Close()
	hasher := sha256.New()
	written, err := io.Copy(hasher, file)
	if err != nil {
		return 0, "", err
	}
	return written, hex.EncodeToString(hasher.Sum(nil)), nil
}

func copyFile(source string, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open %s: %w", source, err)
	}
	defer input.Close()
	output, err := os.Create(target)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
