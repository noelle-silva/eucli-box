package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

const protocolFileName = "fast-window-dev-protocol.json"

type protocol struct {
	Actions map[string]string `json:"actions"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 {
		return errors.New("用法：fast-window-dev-tool <动作代号>")
	}
	action := args[0]
	parsed, err := loadProtocol()
	if err != nil {
		return err
	}
	command, ok := parsed.Actions[action]
	if !ok {
		return fmt.Errorf("协议未定义动作 %q（可用动作：%s）", action, strings.Join(actionNames(parsed.Actions), ", "))
	}
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("协议动作 %q 的命令为空", action)
	}
	return execute(command)
}

func loadProtocol() (protocol, error) {
	data, err := os.ReadFile(protocolFileName)
	if err != nil {
		return protocol{}, fmt.Errorf("读取协议文件失败：%w", err)
	}
	var parsed protocol
	if err := json.Unmarshal(data, &parsed); err != nil {
		return protocol{}, fmt.Errorf("解析协议文件失败：%w", err)
	}
	return parsed, nil
}

func actionNames(actions map[string]string) []string {
	names := make([]string, 0, len(actions))
	for name := range actions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func execute(command string) error {
	shell, flag := shellCommand()
	cmd := exec.Command(shell, flag, command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("执行动作失败：%w", err)
	}
	return nil
}

func shellCommand() (string, string) {
	if runtime.GOOS == "windows" {
		return "cmd", "/c"
	}
	return "sh", "-c"
}
