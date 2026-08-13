package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"devtools/common/releaseverify"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("verify-background-access", flag.ContinueOnError)
	rootValue := flags.String("root", ".", "repository root")
	runRoot := flags.String("run-root", "", "isolated verification run root")
	mode := flags.String("mode", "", "stage mode")
	if err := flags.Parse(args); err != nil {
		return err
	}
	root, err := filepath.Abs(strings.TrimSpace(*rootValue))
	if err != nil {
		return err
	}
	if strings.TrimSpace(*runRoot) == "" {
		return fmt.Errorf("必须指定 -run-root")
	}
	return releaseverify.BackgroundAccess(ctx, root, *runRoot, *mode)
}
