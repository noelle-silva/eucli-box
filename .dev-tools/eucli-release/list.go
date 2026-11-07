package main

import (
	"flag"
	"fmt"

	"devtools/common/artifactdiscovery"
	"eucli-box/pkg/releasecatalog"
)

func runList(args []string) error {
	flags := flag.NewFlagSet("eucli-release list", flag.ContinueOnError)
	root := flags.String("root", ".", "仓库根目录")
	if err := flags.Parse(args); err != nil {
		return err
	}
	repositoryRoot, err := repositoryRoot(*root)
	if err != nil {
		return err
	}
	sources, err := releasecatalog.LoadSources()
	if err != nil {
		return err
	}
	identities, err := artifactdiscovery.Discover(repositoryRoot)
	if err != nil {
		return err
	}
	for _, identity := range identities {
		source, err := sources.SourceFor(identity.Kind)
		if err != nil {
			return err
		}
		fmt.Printf("%s\t%s\n", releasecatalog.Target(identity), source.Repository)
	}
	return nil
}
