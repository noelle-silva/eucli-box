package releasecredentials

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/pkg/types"
	"eucli-box/pkg/workspace"
)

const relativePath = workspace.RelativeCredentialsPath

var keyByKind = map[string]string{
	types.ReleaseArtifactKindBox:    "EUCLI_BOX_GITHUB_TOKEN",
	types.ReleaseArtifactKindTool:   "EUCLI_TOOLS_GITHUB_TOKEN",
	types.ReleaseArtifactKindPlugin: "EUCLI_PLUGINS_GITHUB_TOKEN",
}

type Credentials struct {
	values map[string]string
}

func Load(repositoryRoot string) (Credentials, error) {
	root, err := filepath.Abs(strings.TrimSpace(repositoryRoot))
	if err != nil || strings.TrimSpace(repositoryRoot) == "" {
		return Credentials{}, fmt.Errorf("GitHub 凭据所在的项目根目录无效")
	}
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Credentials{}, fmt.Errorf("缺少本地发布凭据文件 %s", relativePath)
		}
		return Credentials{}, fmt.Errorf("读取 GitHub 凭据文件失败：%w", err)
	}
	if !info.Mode().IsRegular() {
		return Credentials{}, fmt.Errorf("GitHub 凭据文件必须是普通文件")
	}
	file, err := os.Open(path)
	if err != nil {
		return Credentials{}, fmt.Errorf("打开 GitHub 凭据文件失败：%w", err)
	}
	defer file.Close()

	allowed := make(map[string]struct{}, len(keyByKind))
	for _, key := range keyByKind {
		allowed[key] = struct{}{}
	}
	values := make(map[string]string, len(allowed))
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !ok || key == "" {
			return Credentials{}, fmt.Errorf("%s 第 %d 行格式无效，必须使用 NAME=value", relativePath, lineNumber)
		}
		if _, ok := allowed[key]; !ok {
			return Credentials{}, fmt.Errorf("%s 第 %d 行包含未知字段 %s", relativePath, lineNumber, key)
		}
		if _, exists := values[key]; exists {
			return Credentials{}, fmt.Errorf("%s 包含重复字段 %s", relativePath, key)
		}
		if strings.ContainsAny(value, "\"' \t") {
			return Credentials{}, fmt.Errorf("%s 中的 %s 必须直接填写凭据，不使用引号或空格", relativePath, key)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return Credentials{}, fmt.Errorf("读取 GitHub 凭据文件失败：%w", err)
	}
	for _, key := range keyByKind {
		if _, exists := values[key]; !exists {
			return Credentials{}, fmt.Errorf("%s 缺少字段 %s", relativePath, key)
		}
	}
	return Credentials{values: values}, nil
}

func (c Credentials) TokenFor(kind string) (string, error) {
	key, ok := keyByKind[strings.TrimSpace(kind)]
	if !ok {
		return "", fmt.Errorf("未知的正式发行类别 %q", kind)
	}
	token := strings.TrimSpace(c.values[key])
	if token == "" {
		return "", fmt.Errorf("%s 中的 %s 尚未填写", relativePath, key)
	}
	return token, nil
}
