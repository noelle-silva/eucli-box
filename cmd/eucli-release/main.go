package main

import (
	"context"
	"fmt"
	"os"
	"strings"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError()
	}
	command := strings.TrimSpace(args[0])
	switch command {
	case "build":
		return runBuild(ctx, args[1:])
	case "publish":
		return runPublish(ctx, args[1:])
	case "remote":
		return runRemote(ctx, args[1:])
	case "list":
		return runList(args[1:])
	default:
		return usageError()
	}
}

func usageError() error {
	return fmt.Errorf("用法：eucli-release <build|publish|remote|list> [参数]")
}
