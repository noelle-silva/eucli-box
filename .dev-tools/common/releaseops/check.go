package releaseops

import (
	"fmt"
	"os"
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
