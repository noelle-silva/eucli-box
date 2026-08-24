package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"devtools/general-verification-tools/verify-command-execution-limit-protection/commandexecutionlimitprotectionverify"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("verify-command-execution-limit-protection", flag.ContinueOnError)
	rootValue := flags.String("root", "", "repository root")
	runRootValue := flags.String("run-root", "", "isolated verification run root")
	modeValue := flags.String("mode", "", "verification mode")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*rootValue) == "" {
		return fmt.Errorf("must specify -root")
	}
	if strings.TrimSpace(*runRootValue) == "" {
		return fmt.Errorf("must specify -run-root")
	}
	return verify.Run(ctx, *rootValue, *runRootValue, *modeValue)
}
