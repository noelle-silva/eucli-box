package releaseartifact

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"eucli-box/pkg/release"
)

// NextDevelopmentVersion 依据某发布物的历史成品目录决定下一个四段开发版本：
// 基线取自源码正式版本，尾号取同基线历史最大尾号的下一号；
// 基线升级后同基线历史为空，尾号从 .1 重新起步。
// productRoot 是该发布物的版本目录集合（直接包含 <版本>/ 子目录的货物目录）。
func NextDevelopmentVersion(productRoot string, sourceVersion string) (string, error) {
	sourceVersion = strings.TrimSpace(sourceVersion)
	if err := release.ValidateFormalVersion(sourceVersion); err != nil {
		return "", fmt.Errorf("源码版本无效：%w", err)
	}
	maxTail, err := maxDevelopmentTail(productRoot, sourceVersion)
	if err != nil {
		return "", err
	}
	return sourceVersion + "." + strconv.Itoa(maxTail+1), nil
}

// maxDevelopmentTail 扫描该发布物的版本目录，返回同基线四段版本的最大尾号；没有则 0。
func maxDevelopmentTail(productRoot string, sourceVersion string) (int, error) {
	entries, err := os.ReadDir(productRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("读取发布物成品目录失败：%w", err)
	}
	prefix := sourceVersion + "."
	maxTail := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		version := entry.Name()
		if !strings.HasPrefix(version, prefix) {
			continue
		}
		fourth := strings.TrimPrefix(version, prefix)
		if fourth == "" || strings.Contains(fourth, ".") {
			continue
		}
		tail, err := strconv.Atoi(fourth)
		if err != nil || tail < 1 {
			continue
		}
		if tail > maxTail {
			maxTail = tail
		}
	}
	return maxTail, nil
}
