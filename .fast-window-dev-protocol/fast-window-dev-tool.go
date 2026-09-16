package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

const protocolFileName = "fast-window-dev-protocol.json"

const (
	contractVersion = 1
	receiptPrefix   = "FAST-WINDOW-DEV-RECEIPT: "

	statusSucceeded = "succeeded"
	statusFailed    = "failed"
)

type actionDefinition struct {
	Command  string            `json:"command"`
	Artifact map[string]string `json:"artifact"`
}

// UnmarshalJSON 同时接受两种动作形态：纯命令字符串，或带产物声明的对象。
func (definition *actionDefinition) UnmarshalJSON(payload []byte) error {
	var text string
	if err := json.Unmarshal(payload, &text); err == nil {
		definition.Command = text
		return nil
	}
	var parsed struct {
		Command  string            `json:"command"`
		Artifact map[string]string `json:"artifact"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return err
	}
	definition.Command = parsed.Command
	definition.Artifact = parsed.Artifact
	return nil
}

type protocol struct {
	Actions map[string]actionDefinition `json:"actions"`
}

type receiptData struct {
	Result   any               `json:"result"`
	Artifact map[string]string `json:"artifact"`
}

type receipt struct {
	ContractVersion int         `json:"contractVersion"`
	Action          string      `json:"action"`
	Status          string      `json:"status"`
	ExitCode        *int        `json:"exitCode"`
	Error           string      `json:"error"`
	Data            receiptData `json:"data"`
}

func main() {
	os.Exit(runMain(os.Args[1:], protocolFileName, os.Stdout))
}

func runMain(args []string, protocolFile string, output io.Writer) int {
	result := executeAction(args, protocolFile)
	if err := writeReceipt(output, result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if result.Status == statusSucceeded {
		return 0
	}
	return 1
}

func executeAction(args []string, protocolFile string) receipt {
	if len(args) != 1 {
		return failureReceipt("", errors.New("用法：fast-window-dev-tool <动作代号>"))
	}
	action := strings.TrimSpace(args[0])
	parsed, err := readProtocol(protocolFile)
	if err != nil {
		return failureReceipt(action, err)
	}
	definition, ok := parsed.Actions[action]
	if !ok {
		return failureReceipt(action, fmt.Errorf("协议未定义动作 %q（可用动作：%s）", action, strings.Join(actionNames(parsed.Actions), ", ")))
	}
	if strings.TrimSpace(definition.Command) == "" {
		return failureReceipt(action, fmt.Errorf("协议动作 %q 的命令为空", action))
	}
	exitCode, result, artifact, err := runCommand(definition)
	if err != nil {
		return failureReceipt(action, err)
	}
	status := statusSucceeded
	if exitCode != 0 {
		status = statusFailed
	}
	return receipt{
		ContractVersion: contractVersion,
		Action:          action,
		Status:          status,
		ExitCode:        &exitCode,
		Data: receiptData{
			Result:   result,
			Artifact: artifact,
		},
	}
}

func failureReceipt(action string, cause error) receipt {
	return receipt{
		ContractVersion: contractVersion,
		Action:          action,
		Status:          statusFailed,
		Error:           cause.Error(),
		Data:            receiptData{Artifact: map[string]string{}},
	}
}

func readProtocol(file string) (protocol, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return protocol{}, fmt.Errorf("读取协议文件失败：%w", err)
	}
	var parsed protocol
	if err := json.Unmarshal(data, &parsed); err != nil {
		return protocol{}, fmt.Errorf("解析协议文件失败：%w", err)
	}
	return parsed, nil
}

func actionNames(actions map[string]actionDefinition) []string {
	names := make([]string, 0, len(actions))
	for name := range actions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func runCommand(definition actionDefinition) (int, any, map[string]string, error) {
	var captured bytes.Buffer
	exitCode, err := runShellCommand(definition.Command, &captured)
	if err != nil {
		return 0, nil, nil, err
	}
	result := parseJSONOutput(captured.Bytes())
	return exitCode, result, extractArtifact(definition.Artifact, result), nil
}

// runShellCommand 执行 shell 命令并捕获标准输出；Windows 上命令原文写入临时脚本再执行，
// 避免命令行参数转义破坏引号等 shell 语法。
func runShellCommand(source string, captured *bytes.Buffer) (int, error) {
	var command *exec.Cmd
	cleanup := func() {}
	if runtime.GOOS == "windows" {
		scriptPath, err := writeCommandScript(source)
		if err != nil {
			return 0, err
		}
		cleanup = func() { _ = os.Remove(scriptPath) }
		command = exec.Command("cmd", "/c", scriptPath)
	} else {
		command = exec.Command("sh", "-c", source)
	}
	defer cleanup()
	command.Stdout = io.MultiWriter(os.Stdout, captured)
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			return 0, fmt.Errorf("执行动作失败：%w", err)
		}
	}
	return command.ProcessState.ExitCode(), nil
}

func writeCommandScript(source string) (string, error) {
	file, err := os.CreateTemp("", "fast-window-dev-command-*.cmd")
	if err != nil {
		return "", fmt.Errorf("创建命令脚本失败：%w", err)
	}
	path := file.Name()
	if _, err := file.WriteString("@echo off\r\n" + source + "\r\n"); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("写入命令脚本失败：%w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("关闭命令脚本失败：%w", err)
	}
	return path, nil
}

func parseJSONOutput(payload []byte) any {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return nil
	}
	var parsed any
	if err := json.Unmarshal(trimmed, &parsed); err != nil {
		return nil
	}
	return parsed
}

func extractArtifact(declaration map[string]string, result any) map[string]string {
	artifact := map[string]string{}
	for field, sourcePath := range declaration {
		if value, ok := valueAtPath(result, sourcePath); ok {
			artifact[field] = value
		}
	}
	return artifact
}

func valueAtPath(root any, dotted string) (string, bool) {
	current := root
	for _, segment := range strings.Split(dotted, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		value, ok := object[segment]
		if !ok {
			return "", false
		}
		current = value
	}
	text, ok := current.(string)
	if !ok {
		return "", false
	}
	return text, true
}

func writeReceipt(output io.Writer, value receipt) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("生成回执失败：%w", err)
	}
	_, err = fmt.Fprintln(output, receiptPrefix+string(payload))
	return err
}
