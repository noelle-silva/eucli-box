package main

import (
	"flag"
	"fmt"

	"eucli-box/pkg/releasecatalog"
)

func runList(args []string) error {
	flags := flag.NewFlagSet("eucli-release list", flag.ContinueOnError)
	if err := flags.Parse(args); err != nil {
		return err
	}
	catalog, err := releasecatalog.Load()
	if err != nil {
		return err
	}
	for _, identity := range catalog.SortedArtifacts() {
		source, err := catalog.SourceFor(identity.Kind)
		if err != nil {
			return err
		}
		fmt.Printf("%s\t%s\n", releasecatalog.Target(identity), source.Repository)
	}
	return nil
}
