// 命令 build 是 AI 工具的本地开发构建入口。
//
// 在任意工具目录里运行：
//
//	go run devtools/build
//
// 即把当前目录对应的工具构建为成品，并按四段开发版本铺入本地开发货架。
// 额外参数会原样转交给底层铺货命令，例如：
//
//	go run devtools/build -version 0.2.0.9
//
// 本入口不向任何工具目录写入文件，工具身份完全来自该工具自己的 tool.json；
// 它只把仓库既有的铺货命令以最浅的方式暴露出来，不重复任何构建逻辑。
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "构建失败："+err.Error())
		os.Exit(1)
	}
}

func run(extraArgs []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("获取当前目录失败：%w", err)
	}
	toolID, err := readToolID(cwd)
	if err != nil {
		return err
	}
	root, err := findRepositoryRoot(cwd)
	if err != nil {
		return err
	}
	args := append([]string{"run", "devtools/eucli-store-sync", "-target", "tool:" + toolID}, extraArgs...)
	command := exec.Command("go", args...)
	command.Dir = filepath.Join(root, ".dev-tools")
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

// readToolID 从当前目录的 tool.json 读取工具身份，并确认当前目录确是一个工具目录。
func readToolID(dir string) (string, error) {
	payload, err := os.ReadFile(filepath.Join(dir, "tool.json"))
	if err != nil {
		return "", fmt.Errorf("当前目录不是 AI 工具目录（缺少 tool.json）：请在工具目录内运行 go run devtools/build")
	}
	var definition struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload, &definition); err != nil {
		return "", fmt.Errorf("解析 tool.json 失败：%w", err)
	}
	id := strings.TrimSpace(definition.ID)
	if id == "" {
		return "", fmt.Errorf("tool.json 缺少 id 字段")
	}
	if id != filepath.Base(dir) {
		return "", fmt.Errorf("tool.json 的 id（%s）与目录名（%s）不一致", id, filepath.Base(dir))
	}
	return id, nil
}

// findRepositoryRoot 从起始目录向上查找主仓库根：需同时存在 tools 目录与 .dev-tools/go.mod。
func findRepositoryRoot(start string) (string, error) {
	current := start
	for {
		if isRepositoryRoot(current) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("未找到主仓库根（需同时存在 tools 目录与 .dev-tools/go.mod）")
		}
		current = parent
	}
}

func isRepositoryRoot(dir string) bool {
	if info, err := os.Stat(filepath.Join(dir, "tools")); err != nil || !info.IsDir() {
		return false
	}
	if info, err := os.Stat(filepath.Join(dir, ".dev-tools", "go.mod")); err != nil || info.IsDir() {
		return false
	}
	return true
}
