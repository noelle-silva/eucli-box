package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/internal/releaseasset"
	"eucli-box/pkg/releasecatalog"
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
		return err
	}
	base := filepath.Join(repositoryRoot, ".release", "assets")
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
