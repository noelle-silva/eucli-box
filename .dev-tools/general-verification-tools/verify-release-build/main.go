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
	flags := flag.NewFlagSet("verify-release-build", flag.ContinueOnError)
	rootValue := flags.String("root", ".", "repository root")
	runRoot := flags.String("run-root", "", "isolated verification run root")
	modeValue := flags.String("mode", "", "stage mode")
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
	mode := strings.TrimSpace(*modeValue)
	if mode != "" && mode != "full" {
		return fmt.Errorf("正式成品制作验证模式只接受 full")
	}
	return releaseverify.VerifyReleaseBuild(ctx, root, *runRoot)
}
