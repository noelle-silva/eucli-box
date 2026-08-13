// migrationharness 是阶段六验证的迁移替身程序：登记测试迁移步骤后
// 走真实迁移系统（eucli-box/src/data-migration-system）的 Prepare/Run/Complete。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	datamigration "eucli-box/src/data-migration-system"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("migrationharness", flag.ContinueOnError)
	dataDir := flags.String("data-dir", "", "isolated data directory")
	target := flags.String("target", "", "target data version")
	chain := flags.String("chain", "ok", "test chain: ok or gap")
	failAt := flags.Int("fail-at", 0, "step whose Apply fails (1-based)")
	verifyFailAt := flags.Int("verify-fail-at", 0, "step whose Verify fails (1-based)")
	crashAt := flags.Int("crash-at", 0, "step whose Apply crashes the process (1-based)")
	corruptBackup := flags.Bool("corrupt-backup", false, "corrupt backup manifest before Run")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*dataDir) == "" || strings.TrimSpace(*target) == "" {
		return fmt.Errorf("-data-dir 与 -target 必填")
	}
	if *chain != "ok" && *chain != "gap" {
		return fmt.Errorf("-chain 只接受 ok 或 gap")
	}
	registerHarnessSteps(*chain, *failAt, *verifyFailAt, *crashAt)
	session, err := datamigration.Prepare(ctx, *dataDir, *target)
	if err != nil {
		return err
	}
	if *corruptBackup {
		if err := corruptLatestBackup(*dataDir); err != nil {
			return err
		}
	}
	if err := session.Run(ctx); err != nil {
		return err
	}
	if err := session.Complete(ctx); err != nil {
		return err
	}
	outcome := session.Outcome()
	payload, err := json.Marshal(map[string]string{
		"state":  string(outcome.State),
		"from":   outcome.From,
		"to":     outcome.To,
		"detail": outcome.Detail,
	})
	if err != nil {
		return err
	}
	fmt.Println(string(payload))
	return nil
}

// registerHarnessSteps 登记测试迁移链；gap 只登记第一步。
func registerHarnessSteps(chain string, failAt int, verifyFailAt int, crashAt int) {
	register := datamigration.Register
	register(stepOne(failAt, verifyFailAt, crashAt))
	if chain == "ok" {
		register(stepTwo(failAt, verifyFailAt, crashAt))
	}
}

func stepOne(failAt int, verifyFailAt int, crashAt int) datamigration.Step {
	return datamigration.Step{
		ID:          "1.0.0-to-1.1.0",
		FromVersion: "1.0.0",
		ToVersion:   "1.1.0",
		Scope:       []string{"meta/counter.json"},
		Precheck: func(ctx context.Context, dataDir string) error {
			count, err := readCounter(dataDir)
			if err != nil {
				return err
			}
			if count != 0 {
				return fmt.Errorf("counter 必须为 0 才能执行第一步")
			}
			return nil
		},
		Apply: func(ctx context.Context, dataDir string) error {
			if crashAt == 1 {
				if err := writeCounter(dataDir, 1); err != nil {
					return err
				}
				os.Exit(17)
			}
			if failAt == 1 {
				return fmt.Errorf("注入第一步执行失败")
			}
			return writeCounter(dataDir, 1)
		},
		Verify: func(ctx context.Context, dataDir string) error {
			if verifyFailAt == 1 {
				return fmt.Errorf("注入第一步核对失败")
			}
			count, err := readCounter(dataDir)
			if err != nil {
				return err
			}
			if count != 1 {
				return fmt.Errorf("第一步后 counter 必须为 1")
			}
			return nil
		},
	}
}

func stepTwo(failAt int, verifyFailAt int, crashAt int) datamigration.Step {
	return datamigration.Step{
		ID:          "1.1.0-to-1.2.0",
		FromVersion: "1.1.0",
		ToVersion:   "1.2.0",
		Scope:       []string{"meta/counter.json", "meta/stamp.json"},
		Precheck: func(ctx context.Context, dataDir string) error {
			count, err := readCounter(dataDir)
			if err != nil {
				return err
			}
			if count != 1 {
				return fmt.Errorf("counter 必须为 1 才能执行第二步")
			}
			return nil
		},
		Apply: func(ctx context.Context, dataDir string) error {
			if crashAt == 2 {
				if err := writeCounter(dataDir, 2); err != nil {
					return err
				}
				os.Exit(17)
			}
			if failAt == 2 {
				return fmt.Errorf("注入第二步执行失败")
			}
			if err := writeCounter(dataDir, 2); err != nil {
				return err
			}
			return writeStamp(dataDir, "1.2.0")
		},
		Verify: func(ctx context.Context, dataDir string) error {
			if verifyFailAt == 2 {
				return fmt.Errorf("注入第二步核对失败")
			}
			count, err := readCounter(dataDir)
			if err != nil {
				return err
			}
			if count != 2 {
				return fmt.Errorf("第二步后 counter 必须为 2")
			}
			stamp, err := readStamp(dataDir)
			if err != nil {
				return err
			}
			if stamp != "1.2.0" {
				return fmt.Errorf("第二步后 stamp 必须为 1.2.0")
			}
			return nil
		},
	}
}

// corruptLatestBackup 把最新备份清单替换为无效 JSON，用于制造恢复失败。
func corruptLatestBackup(dataDir string) error {
	backupRoot := filepath.Join(datamigration.WorkspaceDir(dataDir), "backup")
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		return fmt.Errorf("读取备份目录失败：%w", err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("备份目录为空，无法制造损坏")
	}
	manifestPath := filepath.Join(backupRoot, entries[0].Name(), "manifest.json")
	if err := os.WriteFile(manifestPath, []byte("{corrupt"), 0o644); err != nil {
		return fmt.Errorf("写入损坏清单失败：%w", err)
	}
	return nil
}

func readCounter(dataDir string) (int, error) {
	payload, err := os.ReadFile(filepath.Join(dataDir, "meta", "counter.json"))
	if err != nil {
		return 0, err
	}
	var value struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(payload, &value); err != nil {
		return 0, err
	}
	return value.Count, nil
}

func writeCounter(dataDir string, count int) error {
	payload, err := json.MarshalIndent(map[string]any{"count": count}, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(dataDir, "meta", "counter.json"), payload)
}

func readStamp(dataDir string) (string, error) {
	payload, err := os.ReadFile(filepath.Join(dataDir, "meta", "stamp.json"))
	if err != nil {
		return "", err
	}
	var value struct {
		Stamp string `json:"stamp"`
	}
	if err := json.Unmarshal(payload, &value); err != nil {
		return "", err
	}
	return value.Stamp, nil
}

func writeStamp(dataDir string, stamp string) error {
	payload, err := json.MarshalIndent(map[string]any{"stamp": stamp}, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(dataDir, "meta", "stamp.json"), payload)
}

func writeFile(path string, payload []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}
