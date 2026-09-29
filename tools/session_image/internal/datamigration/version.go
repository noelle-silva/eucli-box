package datamigration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// versionFileName 是发布物数据版本事实在数据目录内的约定文件位置。
const versionFileName = "data-version.json"

// dataVersion 是发布物数据版本事实：迁移系统在数据目录内维护的唯一版本记录。
type dataVersion struct {
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// versionFilePath 返回数据版本事实文件的完整路径。
func versionFilePath(dataDir string) string {
	return filepath.Join(dataDir, versionFileName)
}

// loadDataVersion 读取发布物数据版本事实；文件不存在返回零值与 false。
func loadDataVersion(ctx context.Context, dataDir string) (dataVersion, bool, error) {
	if err := ctx.Err(); err != nil {
		return dataVersion{}, false, migrationPrepareFailed("read cancelled", err)
	}
	payload, err := os.ReadFile(versionFilePath(dataDir))
	if err != nil {
		if os.IsNotExist(err) {
			return dataVersion{}, false, nil
		}
		return dataVersion{}, false, migrationPrepareFailed("failed to read data version file", err)
	}
	var version dataVersion
	if err := json.Unmarshal(payload, &version); err != nil {
		return dataVersion{}, false, migrationPrepareFailed("failed to decode data version file", err)
	}
	if err := validateVersion(version.Version); err != nil {
		return dataVersion{}, false, migrationPrepareFailed("data version is invalid", err)
	}
	return version, true, nil
}

// saveDataVersion 以原子替换方式写入发布物数据版本事实。
func saveDataVersion(ctx context.Context, dataDir string, version dataVersion) error {
	if err := validateVersion(version.Version); err != nil {
		return migrationPrepareFailed("refusing to write invalid data version", err)
	}
	if err := ctx.Err(); err != nil {
		return migrationPrepareFailed("write cancelled", err)
	}
	return writeJSONAtomic(versionFilePath(dataDir), version)
}

// semanticVersion 是主.次.补( [.开发序号] )版本。
type semanticVersion struct {
	major    int
	minor    int
	patch    int
	build    int
	hasBuild bool
}

// validateVersion 校验三段正式版本或四段开发版本。
func validateVersion(value string) error {
	_, err := parseVersion(value)
	return err
}

// compareVersions 比较两个版本号：左小返回 -1，相等返回 0，左大返回 1。
func compareVersions(left string, right string) (int, error) {
	leftVersion, err := parseVersion(left)
	if err != nil {
		return 0, fmt.Errorf("左侧版本无效：%w", err)
	}
	rightVersion, err := parseVersion(right)
	if err != nil {
		return 0, fmt.Errorf("右侧版本无效：%w", err)
	}
	return compare(leftVersion, rightVersion), nil
}

func parseVersion(value string) (semanticVersion, error) {
	trimmed := strings.TrimSpace(value)
	parts := strings.Split(trimmed, ".")
	if len(parts) != 3 && len(parts) != 4 {
		return semanticVersion{}, fmt.Errorf("版本必须使用三段正式版本或四段开发版本，例如 0.1.0 或 0.1.0.1")
	}
	ints := make([]int, len(parts))
	for index, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return semanticVersion{}, fmt.Errorf("版本必须使用三段正式版本或四段开发版本，例如 0.1.0 或 0.1.0.1")
		}
		parsed, err := strconv.Atoi(part)
		if err != nil || parsed < 0 {
			return semanticVersion{}, fmt.Errorf("版本必须使用三段正式版本或四段开发版本，例如 0.1.0 或 0.1.0.1")
		}
		ints[index] = parsed
	}
	return semanticVersion{
		major:    ints[0],
		minor:    ints[1],
		patch:    ints[2],
		build:    optionalInt(ints, 3),
		hasBuild: len(parts) == 4,
	}, nil
}

func optionalInt(values []int, index int) int {
	if index >= len(values) {
		return 0
	}
	return values[index]
}

func compare(left semanticVersion, right semanticVersion) int {
	if left.major != right.major {
		if left.major < right.major {
			return -1
		}
		return 1
	}
	if left.minor != right.minor {
		if left.minor < right.minor {
			return -1
		}
		return 1
	}
	if left.patch < right.patch {
		return -1
	}
	if left.patch > right.patch {
		return 1
	}
	if left.build < right.build {
		return -1
	}
	if left.build > right.build {
		return 1
	}
	return 0
}
