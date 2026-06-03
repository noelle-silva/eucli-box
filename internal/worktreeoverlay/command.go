package worktreeoverlay

import (
	"context"
	"fmt"
	"io"
	"os"
)

type Options struct {
	WorkDir string
	Stdout  io.Writer
	Stderr  io.Writer
}

func Run(ctx context.Context, args []string, opts Options) error {
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	if opts.WorkDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
		opts.WorkDir = cwd
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printUsage(opts.Stdout)
		return nil
	}
	runtime, err := newRuntime(ctx, opts.WorkDir)
	if err != nil {
		return err
	}
	switch args[0] {
	case "apply":
		if len(args) != 2 {
			return fmt.Errorf("usage: worktree-overlay apply <worktree-name-or-path>")
		}
		return runtime.Apply(args[1], opts.Stdout)
	case "refresh":
		if len(args) > 2 {
			return fmt.Errorf("usage: worktree-overlay refresh [worktree-name-or-path]")
		}
		sourceSpec := ""
		if len(args) == 2 {
			sourceSpec = args[1]
		}
		return runtime.Refresh(sourceSpec, opts.Stdout)
	case "status":
		if len(args) != 1 {
			return fmt.Errorf("usage: worktree-overlay status")
		}
		return runtime.Status(opts.Stdout)
	case "clear":
		if len(args) != 1 {
			return fmt.Errorf("usage: worktree-overlay clear")
		}
		return runtime.Clear(opts.Stdout)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage(output io.Writer) {
	fmt.Fprint(output, `worktree-overlay applies a worktree change set onto the current worktree for local testing.

Usage:
  worktree-overlay apply <worktree-name-or-path>
  worktree-overlay refresh [worktree-name-or-path]
  worktree-overlay status
  worktree-overlay clear

Workflow:
  1. Edit only in the source worktree.
  2. Run apply once from the target worktree.
  3. Keep testing in the target worktree with fixed commands.
  4. Run refresh after source worktree changes.
  5. Run clear only when the overlay is no longer needed.

Notes:
  - Bare names resolve to .worktrees/<name> first.
  - The target worktree must be clean before applying.
  - refresh replaces the active overlay internally; manual clear is not required.
  - clear accepts files that are still overlaid or already restored to original content.
`)
}
