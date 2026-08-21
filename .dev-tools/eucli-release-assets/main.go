package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"devtools/common/releaseasset"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
	"eucli-box/pkg/workspace"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("eucli-release-assets", flag.ContinueOnError)
	target := flags.String("target", "", "release target")
	root := flags.String("root", ".", "repository root")
	output := flags.String("output", "", "prepared asset output root")
	cache := flags.String("cache", "", "download cache root")
	temp := flags.String("temp", "", "temporary extraction root")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*target) == "" {
		return fmt.Errorf("必须指定 -target")
	}
	repositoryRoot, err := filepath.Abs(strings.TrimSpace(*root))
	if err != nil {
		return err
	}
	catalog, err := releasecatalog.Load()
	if err != nil {
		return err
	}
	identity, err := catalog.ResolveTarget(*target)
	if err != nil {
		// 资产库按配方服务任意发布物；正式发行白名单未收录时，
		// 允许按 kind:id 直接解析工具发布物，只要配方中存在对应项。
		kind, id, ok := strings.Cut(strings.TrimSpace(*target), ":")
		if !ok || strings.TrimSpace(kind) != types.ReleaseArtifactKindTool || strings.TrimSpace(id) == "" {
			return err
		}
		direct := types.ReleaseArtifactIdentity{Kind: strings.TrimSpace(kind), ID: strings.TrimSpace(id)}
		if len(assetRecipesFor(direct)) == 0 {
			return err
		}
		identity = direct
	}
	base := workspace.AssetRoot(repositoryRoot)
	if strings.TrimSpace(*output) == "" {
		*output = filepath.Join(base, "prepared")
	}
	if strings.TrimSpace(*cache) == "" {
		*cache = filepath.Join(base, "cache")
	}
	if strings.TrimSpace(*temp) == "" {
		*temp = filepath.Join(base, "temp")
	}
	prepared, err := releaseasset.PrepareRequired(ctx, releaseasset.PrepareOptions{
		RepositoryRoot: repositoryRoot,
		Artifact:       identity,
		OutputRoot:     *output,
		CacheRoot:      *cache,
		TempRoot:       *temp,
	})
	if err != nil {
		return err
	}
	if len(prepared) == 0 {
		fmt.Printf("%s 不需要外部随包内容\n", releasecatalog.Target(identity))
		return nil
	}
	for name, path := range prepared {
		fmt.Printf("%s=%s\n", name, path)
	}
	return nil
}

// assetRecipesFor 绕过白名单判断某发布物是否存在外部配方。
func assetRecipesFor(identity types.ReleaseArtifactIdentity) []releaseasset.Recipe {
	catalog, err := releaseasset.LoadCatalog()
	if err != nil {
		return nil
	}
	return catalog.RecipesForArtifact(identity)
}
