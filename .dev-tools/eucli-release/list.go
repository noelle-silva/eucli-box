package main

import (
	"flag"
	"fmt"

	"eucli-box/pkg/artifactcatalog"
	"eucli-box/pkg/releasecatalog"
)

func runList(args []string) error {
	flags := flag.NewFlagSet("eucli-release list", flag.ContinueOnError)
	if err := flags.Parse(args); err != nil {
		return err
	}
	sources, err := releasecatalog.LoadSources()
	if err != nil {
		return err
	}
	roster, err := artifactcatalog.Load()
	if err != nil {
		return err
	}
	for _, identity := range roster.SortedArtifacts() {
		source, err := sources.SourceFor(identity.Kind)
		if err != nil {
			return err
		}
		fmt.Printf("%s\t%s\n", releasecatalog.Target(identity), source.Repository)
	}
	return nil
}
