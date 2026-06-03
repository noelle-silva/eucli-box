package main

import (
	"context"
	"fmt"
	"os"

	"eucli-box/internal/worktreeoverlay"
)

func main() {
	if err := worktreeoverlay.Run(context.Background(), os.Args[1:], worktreeoverlay.Options{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
