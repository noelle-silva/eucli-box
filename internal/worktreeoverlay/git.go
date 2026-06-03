package worktreeoverlay

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type runtimeContext struct {
	ctx       context.Context
	target    repository
	stateRoot string
}

type repository struct {
	Root string
	Head string
}

func newRuntime(ctx context.Context, workDir string) (*runtimeContext, error) {
	root, err := gitOutput(ctx, workDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("resolve target repository root: %w", err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve target repository root: %w", err)
	}
	root = filepath.Clean(root)
	head, err := gitOutput(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("resolve target HEAD: %w", err)
	}
	gitDir, err := gitOutput(ctx, root, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, fmt.Errorf("resolve target git dir: %w", err)
	}
	return &runtimeContext{
		ctx:       ctx,
		target:    repository{Root: root, Head: head},
		stateRoot: filepath.Join(filepath.Clean(gitDir), "worktree-overlay"),
	}, nil
}

func (rt *runtimeContext) resolveSource(spec string) (repository, string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return repository{}, "", fmt.Errorf("source worktree is required")
	}
	candidates := sourceCandidates(rt.target.Root, spec)
	var lastErr error
	for _, candidate := range candidates {
		root, err := gitOutput(rt.ctx, candidate, "rev-parse", "--show-toplevel")
		if err != nil {
			lastErr = err
			continue
		}
		root, err = filepath.Abs(root)
		if err != nil {
			return repository{}, "", fmt.Errorf("resolve source repository root: %w", err)
		}
		root = filepath.Clean(root)
		if samePath(root, rt.target.Root) {
			return repository{}, "", fmt.Errorf("source worktree must differ from target worktree")
		}
		head, err := gitOutput(rt.ctx, root, "rev-parse", "HEAD")
		if err != nil {
			return repository{}, "", fmt.Errorf("resolve source HEAD: %w", err)
		}
		if _, err := gitOutput(rt.ctx, rt.target.Root, "merge-base", rt.target.Head, head); err != nil {
			return repository{}, "", fmt.Errorf("source worktree does not share history with target: %w", err)
		}
		return repository{Root: root, Head: head}, spec, nil
	}
	if lastErr != nil {
		return repository{}, "", fmt.Errorf("resolve source worktree %q: %w", spec, lastErr)
	}
	return repository{}, "", fmt.Errorf("resolve source worktree %q", spec)
}

func sourceCandidates(targetRoot string, spec string) []string {
	if filepath.IsAbs(spec) || strings.ContainsAny(spec, `/\`) || strings.HasPrefix(spec, ".") {
		if filepath.IsAbs(spec) {
			return []string{filepath.Clean(spec)}
		}
		return []string{filepath.Join(targetRoot, spec)}
	}
	return []string{
		filepath.Join(targetRoot, ".worktrees", spec),
		filepath.Join(targetRoot, spec),
	}
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	output, err := gitBytes(ctx, dir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func gitList(ctx context.Context, dir string, args ...string) ([]string, error) {
	output, err := gitBytes(ctx, dir, args...)
	if err != nil {
		return nil, err
	}
	parts := bytes.Split(output, []byte{0})
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		values = append(values, string(part))
	}
	return values, nil
}

func gitBytes(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, message)
	}
	return output, nil
}

func cleanStatus(ctx context.Context, root string) (string, error) {
	return gitOutput(ctx, root, "status", "--porcelain", "--untracked-files=normal")
}

func gitQuiet(ctx context.Context, dir string, args ...string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			return false, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return false, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, message)
	}
	return true, nil
}
