package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"devtools/general-verification-tools/verify-box-update/boxupdateverify"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("verify-box-update", flag.ContinueOnError)
	rootValue := flags.String("root", "", "repository root")
	runRootValue := flags.String("run-root", "", "isolated verification run root")
	modeValue := flags.String("mode", "", "stage mode")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*rootValue) == "" {
		return fmt.Errorf("必须指定 -root")
	}
	if strings.TrimSpace(*runRootValue) == "" {
		return fmt.Errorf("必须指定 -run-root")
	}
	return boxupdateverify.Run(ctx, *rootValue, *runRootValue, *modeValue)
}
