package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"devtools/common/releasepublish"
	"devtools/common/toolruntime"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
)

func runDelist(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("eucli-release delist", flag.ContinueOnError)
	rootValue := flags.String("root", ".", "repository root")
	target := flags.String("target", "", "release target (tool:<id> or plugin:<id>; whitelist not required)")
	version := flags.String("version", "", "specific formal version; empty removes the whole artifact")
	confirmed := flags.Bool("confirm-delist", false, "explicitly allow a real GitHub delist")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*target) == "" {
		return fmt.Errorf("必须指定 -target")
	}
	if !*confirmed {
		return fmt.Errorf("正式下架必须显式提供 -confirm-delist")
	}
	root, err := repositoryRoot(*rootValue)
	if err != nil {
		return err
	}
	catalog, identity, err := resolveDelistTarget(*target)
	if err != nil {
		return err
	}
	source, err := catalog.SourceFor(identity.Kind)
	if err != nil {
		return err
	}
	token, err := githubToken(root, identity.Kind)
	if err != nil {
		return fmt.Errorf("读取正式下架凭据失败：%w", err)
	}
	publisher, err := releasepublish.New(releasepublish.Config{Token: token})
	if err != nil {
		return fmt.Errorf("正式下架准备失败：%w", err)
	}
	result, err := publisher.Delist(ctx, source, releasepublish.DelistInput{Artifact: identity, Version: *version})
	if err != nil {
		return fmt.Errorf("正式下架失败：%w", err)
	}
	if err := toolruntime.WriteScorecard(toolruntime.Root(root, "eucli-release"), "delist", result); err != nil {
		return fmt.Errorf("下架完成，但写入本轮成绩单失败：%w", err)
	}
	return printJSON(result)
}

// resolveDelistTarget 解析下架目标：不经过发布白名单校验，白名单之外的旧发布物同样可以下架。
func resolveDelistTarget(value string) (releasecatalog.Catalog, types.ReleaseArtifactIdentity, error) {
	catalog, err := releasecatalog.Load()
	if err != nil {
		return releasecatalog.Catalog{}, types.ReleaseArtifactIdentity{}, err
	}
	kind, id, ok := strings.Cut(strings.TrimSpace(value), ":")
	if !ok {
		return releasecatalog.Catalog{}, types.ReleaseArtifactIdentity{}, fmt.Errorf("下架目标必须是 tool:<id> 或 plugin:<id>")
	}
	identity := types.ReleaseArtifactIdentity{Kind: strings.TrimSpace(kind), ID: strings.TrimSpace(id)}
	if err := releasecatalog.ValidateArtifactIdentity(identity); err != nil {
		return releasecatalog.Catalog{}, types.ReleaseArtifactIdentity{}, fmt.Errorf("下架目标无效：%w", err)
	}
	return catalog, identity, nil
}
