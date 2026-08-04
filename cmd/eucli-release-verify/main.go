package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/internal/releaseverify"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("必须指定 stage-01、stage-02、stage-03 或 dev-local-box")
	}
	command := strings.TrimSpace(args[0])
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	rootValue := flags.String("root", ".", "repository root")
	runRoot := flags.String("run-root", "", "isolated verification run root")
	mode := flags.String("mode", "", "stage mode")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	root, err := filepath.Abs(strings.TrimSpace(*rootValue))
	if err != nil {
		return err
	}
	if strings.TrimSpace(*runRoot) == "" {
		return fmt.Errorf("必须指定 -run-root")
	}
	switch command {
	case "stage-01":
		return releaseverify.Stage01(ctx, root, *runRoot)
	case "stage-02":
		return releaseverify.Stage02(ctx, root, *runRoot, *mode)
	case "stage-03":
		return releaseverify.Stage03(ctx, root, *runRoot, *mode)
	case "dev-local-box":
		return releaseverify.DevLocalBox(ctx, root, *runRoot, *mode)
	default:
		return fmt.Errorf("未知验证阶段 %q", command)
	}
}
