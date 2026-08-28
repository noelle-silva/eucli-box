package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"devtools/common/toolkit"
)

const (
	toolName = "verify-session-facts-regression"
	version  = "1.0.0"
)

type verificationCommand struct {
	name    string
	command string
	args    []string
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet(toolName, flag.ContinueOnError)
	rootValue := flags.String("root", "", "repository root")
	runRootValue := flags.String("run-root", "", "isolated verification run root")
	modeValue := flags.String("mode", "", "verification mode")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*rootValue) == "" {
		return fmt.Errorf("必须指定 -root")
	}
	if strings.TrimSpace(*runRootValue) == "" {
		return fmt.Errorf("必须指定 -run-root")
	}
	if strings.TrimSpace(*modeValue) != "default" {
		return fmt.Errorf("usage: %s.cmd [default]", toolName)
	}

	run, err := toolkit.PrepareVerificationRun(*rootValue, *runRootValue, toolName)
	if err != nil {
		return err
	}

	recorder := toolkit.NewVerificationRecorder(toolName, *modeValue, run.Root)
	studioRoot := filepath.Join(run.RepositoryRoot, "clients", "eucli-studio")
	backendRoot := filepath.Join(studioRoot, "backend-go")
	for _, directory := range []struct {
		path  string
		label string
	}{
		{path: studioRoot, label: "客户端工程目录"},
		{path: backendRoot, label: "客户端后台工程目录"},
	} {
		if _, err := toolkit.ExistingPlainDirectory(directory.path, directory.label); err != nil {
			return err
		}
	}
	commands := []verificationCommand{
		{
			name:    "frontend regression tests",
			command: "pnpm",
			args:    []string{"--dir", studioRoot, "test", "--", "--reporter=dot"},
		},
		{
			name:    "frontend typecheck",
			command: "pnpm",
			args:    []string{"--dir", studioRoot, "typecheck"},
		},
		{
			name:    "gateway and storage regression tests",
			command: "go",
			args:    []string{"-C", run.RepositoryRoot, "test", "./src/gateway-system", "./src/data-storage-system"},
		},
		{
			name:    "backend projection regression tests",
			command: "go",
			args:    []string{"-C", backendRoot, "test", "./..."},
		},
	}
	for _, item := range commands {
		if err := runCommand(ctx, run, item); err != nil {
			recorder.Fail(item.name, err)
			continue
		}
		recorder.Pass(item.name, "永久回归测试命令通过")
	}

	if err := recorder.Finish(run.Evidence, run.DisposableDirectories()); err != nil {
		return fmt.Errorf("verification failed; report: %s: %w", filepath.Join(run.Evidence, "report.json"), err)
	}
	fmt.Printf("[PASS] %s %s; report: %s\n", toolName, version, filepath.Join(run.Evidence, "report.json"))
	return nil
}

func runCommand(ctx context.Context, run *toolkit.VerificationRun, item verificationCommand) error {
	if _, err := toolkit.ExistingPlainDirectory(run.Work, "验证工具工作目录"); err != nil {
		return err
	}
	return toolkit.RunCommand(ctx, item.name, run.Work, run.Evidence, run.Temp, map[string]string{
		"npm_config_cache": filepath.Join(run.Cache, "npm-cache"),
	}, item.command, item.args...)
}
