package releaseops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

type Result struct {
	Target  string
	Version string
}

func CheckAll(root string) ([]Result, error) {
	artifacts, err := Discover(root)
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(artifacts))
	for _, artifact := range artifacts {
		if err := Check(artifact); err != nil {
			return nil, err
		}
		results = append(results, Result{Target: artifact.Target(), Version: artifact.Version})
	}
	return results, nil
}

func Check(artifact Artifact) error {
	if err := checkChineseDocument(artifact.READMEPath, "README"); err != nil {
		return fmt.Errorf("%s：%w", artifact.Target(), err)
	}
	if err := checkChineseDocument(artifact.ChangelogPath, "CHANGELOG"); err != nil {
		return fmt.Errorf("%s：%w", artifact.Target(), err)
	}
	if err := checkChangelogEntry(artifact); err != nil {
		return fmt.Errorf("%s：%w", artifact.Target(), err)
	}
	if artifact.Kind == KindClient {
		if err := checkClientPackageVersions(artifact); err != nil {
			return fmt.Errorf("%s：%w", artifact.Target(), err)
		}
	}
	return nil
}

// CheckDevelopment 是开发构建使用的发布物检查：保留文档与版本事实检查，
// 但不要求 CHANGELOG 存在当前开发版本条目（开发版本不在源码记录中撰写）。
func CheckDevelopment(artifact Artifact) error {
	if err := checkChineseDocument(artifact.READMEPath, "README"); err != nil {
		return fmt.Errorf("%s：%w", artifact.Target(), err)
	}
	if err := checkChineseDocument(artifact.ChangelogPath, "CHANGELOG"); err != nil {
		return fmt.Errorf("%s：%w", artifact.Target(), err)
	}
	return nil
}

func checkChangelogEntry(artifact Artifact) error {
	payload, err := os.ReadFile(artifact.ChangelogPath)
	if err != nil {
		return fmt.Errorf("读取 CHANGELOG 失败：%w", err)
	}
	heading := regexp.MustCompile(`(?m)^##\s+` + regexp.QuoteMeta(artifact.Version) + `(?:\s|$)`)
	if !heading.Match(payload) {
		return fmt.Errorf("CHANGELOG 缺少当前版本 %s 的记录", artifact.Version)
	}
	return nil
}

func checkChineseDocument(path string, label string) error {
	payload, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取 %s 失败：%w", label, err)
	}
	content := strings.TrimSpace(string(payload))
	if content == "" {
		return fmt.Errorf("%s 不能为空", label)
	}
	hanCount := 0
	latinCount := 0
	for _, value := range content {
		if unicode.Is(unicode.Han, value) {
			hanCount++
		} else if unicode.Is(unicode.Latin, value) {
			latinCount++
		}
	}
	if hanCount < 4 || hanCount*5 < latinCount {
		return fmt.Errorf("%s 必须以中文正文为主", label)
	}
	return nil
}

func checkClientPackageVersions(artifact Artifact) error {
	paths := clientVersionFiles(artifact.Directory)
	for _, path := range paths.jsonFiles {
		payload, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("读取 %s 失败：%w", relativeName(artifact.Directory, path), err)
		}
		var info struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal(payload, &info); err != nil {
			return fmt.Errorf("读取 %s 失败：%w", relativeName(artifact.Directory, path), err)
		}
		if strings.TrimSpace(info.Version) != artifact.Version {
			return fmt.Errorf("%s 的版本与 release.json 不一致", relativeName(artifact.Directory, path))
		}
	}
	for _, source := range []struct {
		path     string
		selector tomlVersionSelector
		name     string
	}{
		{path: paths.cargoTOML, selector: packageVersion, name: "Cargo.toml"},
		{path: paths.cargoLock, selector: eucliStudioLockVersion, name: "Cargo.lock"},
	} {
		payload, err := os.ReadFile(source.path)
		if err != nil {
			return fmt.Errorf("读取 %s 失败：%w", source.name, err)
		}
		version, err := readTOMLVersion(string(payload), source.selector)
		if err != nil {
			return fmt.Errorf("读取 %s 失败：%w", source.name, err)
		}
		if version != artifact.Version {
			return fmt.Errorf("%s 的版本与 release.json 不一致", source.name)
		}
	}
	return nil
}

type clientVersionPaths struct {
	jsonFiles []string
	cargoTOML string
	cargoLock string
}

func clientVersionFiles(directory string) clientVersionPaths {
	return clientVersionPaths{
		jsonFiles: []string{
			filepath.Join(directory, "package.json"),
			filepath.Join(directory, "src-tauri", "tauri.conf.json"),
			filepath.Join(directory, "src-tauri", "tauri.conf.dev.json"),
		},
		cargoTOML: filepath.Join(directory, "src-tauri", "Cargo.toml"),
		cargoLock: filepath.Join(directory, "src-tauri", "Cargo.lock"),
	}
}

type tomlVersionSelector int

const (
	packageVersion tomlVersionSelector = iota
	eucliStudioLockVersion
)

func readTOMLVersion(source string, selector tomlVersionSelector) (string, error) {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	switch selector {
	case packageVersion:
		insidePackage := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "[") {
				insidePackage = trimmed == "[package]"
				continue
			}
			if insidePackage {
				if version, ok := parseTOMLStringAssignment(trimmed, "version"); ok {
					return version, nil
				}
			}
		}
		return "", fmt.Errorf("[package] 中缺少 version")
	case eucliStudioLockVersion:
		insidePackage := false
		matchedName := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "[[package]]" {
				insidePackage = true
				matchedName = false
				continue
			}
			if !insidePackage {
				continue
			}
			if name, ok := parseTOMLStringAssignment(trimmed, "name"); ok {
				matchedName = name == "eucli-studio-app"
				continue
			}
			if matchedName {
				if version, ok := parseTOMLStringAssignment(trimmed, "version"); ok {
					return version, nil
				}
			}
		}
		return "", fmt.Errorf("缺少 eucli-studio-app 包版本")
	default:
		return "", fmt.Errorf("未知 TOML 版本位置")
	}
}

func parseTOMLStringAssignment(line string, key string) (string, bool) {
	prefix := key + " = "
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return "", false
	}
	return value[1 : len(value)-1], true
}

func relativeName(root string, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(relative)
}
