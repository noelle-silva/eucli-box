package releaseartifact

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

// NextDevelopmentVersion 依据构建输出区的历史成品决定下一个四段开发版本：
// 基线取自源码正式版本，尾号取同基线历史最大尾号的下一号；
// 基线升级后同基线历史为空，尾号从 .1 重新起步。
// 构建输出区是成品诞生的第一现场，货架只是从输出区复制过去的副本，不参与版本号决定。
func NextDevelopmentVersion(outputRoot string, identity types.ReleaseArtifactIdentity, sourceVersion string) (string, error) {
	sourceVersion = strings.TrimSpace(sourceVersion)
	if err := release.ValidateFormalVersion(sourceVersion); err != nil {
		return "", fmt.Errorf("源码版本无效：%w", err)
	}
	versionRoot := filepath.Join(outputRoot, outputDirectoryName(identity))
	maxTail, err := maxDevelopmentTail(versionRoot, sourceVersion)
	if err != nil {
		return "", err
	}
	return sourceVersion + "." + strconv.Itoa(maxTail+1), nil
}

// maxDevelopmentTail 扫描输出区该发布物的版本目录，返回同基线四段版本的最大尾号；没有则 0。
func maxDevelopmentTail(versionRoot string, sourceVersion string) (int, error) {
	entries, err := os.ReadDir(versionRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("读取构建输出区失败：%w", err)
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
