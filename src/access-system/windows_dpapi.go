package accesssystem

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// protectSecret 使用当前 Windows 用户的系统保护能力（DPAPI CurrentUser 范围）加密明文内容。
// 输入输出都使用 base64，避免命令行转义问题；失败时明确报错，不退回明文保存。
func protectSecret(ctx context.Context, plainSecret string) (string, error) {
	if runtime.GOOS != "windows" {
		return "", fmt.Errorf("Windows DPAPI 只在 Windows 上可用")
	}
	plainBytes := []byte(plainSecret)
	inputBase64 := base64.StdEncoding.EncodeToString(plainBytes)
	command := `Add-Type -AssemblyName System.Security; ` +
		`$inputBytes = [System.Convert]::FromBase64String('` + inputBase64 + `'); ` +
		`$encrypted = [System.Security.Cryptography.ProtectedData]::Protect($inputBytes, $null, [System.Security.Cryptography.DataProtectionScope]::CurrentUser); ` +
		`[System.Convert]::ToBase64String($encrypted)`
	output, err := runPowerShell(ctx, command)
	if err != nil {
		return "", fmt.Errorf("DPAPI 加密失败：%w", err)
	}
	output = strings.TrimSpace(output)
	if output == "" {
		return "", fmt.Errorf("DPAPI 加密失败：PowerShell 未返回加密结果")
	}
	if _, err := base64.StdEncoding.DecodeString(output); err != nil {
		return "", fmt.Errorf("DPAPI 加密结果无效：%w", err)
	}
	return output, nil
}

// unprotectSecret 使用当前 Windows 用户的系统保护能力（DPAPI CurrentUser 范围）解密内容。
// 失败时明确报错，不返回空值冒充成功。
func unprotectSecret(ctx context.Context, encryptedSecret string) (string, error) {
	if runtime.GOOS != "windows" {
		return "", fmt.Errorf("Windows DPAPI 只在 Windows 上可用")
	}
	encryptedBase64 := strings.TrimSpace(encryptedSecret)
	if _, err := base64.StdEncoding.DecodeString(encryptedBase64); err != nil {
		return "", fmt.Errorf("DPAPI 加密内容无效：%w", err)
	}
	command := `Add-Type -AssemblyName System.Security; ` +
		`$encryptedBytes = [System.Convert]::FromBase64String('` + encryptedBase64 + `'); ` +
		`$plain = [System.Security.Cryptography.ProtectedData]::Unprotect($encryptedBytes, $null, [System.Security.Cryptography.DataProtectionScope]::CurrentUser); ` +
		`[System.Convert]::ToBase64String($plain)`
	output, err := runPowerShell(ctx, command)
	if err != nil {
		return "", fmt.Errorf("DPAPI 解密失败：%w", err)
	}
	output = strings.TrimSpace(output)
	if output == "" {
		return "", fmt.Errorf("DPAPI 解密失败：PowerShell 未返回解密结果")
	}
	plainBytes, err := base64.StdEncoding.DecodeString(output)
	if err != nil {
		return "", fmt.Errorf("DPAPI 解密结果无效：%w", err)
	}
	return string(plainBytes), nil
}

// runPowerShell 执行一段只读取输入、只输出 base64 结果字符串的 PowerShell 命令。
// 输出同时写入 stdout 和 stderr 时视为失败，避免错误信息与结果混淆。
func runPowerShell(ctx context.Context, command string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	executable, err := exec.LookPath("powershell.exe")
	if err != nil {
		return "", fmt.Errorf("找不到 PowerShell：%w", err)
	}
	process := exec.CommandContext(ctx, executable, "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", command)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	process.Stdout = &stdout
	process.Stderr = &stderr
	if err := process.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("PowerShell 执行失败：%s", message)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		return "", fmt.Errorf("PowerShell 输出错误信息：%s", strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
