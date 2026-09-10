package release

import (
	"fmt"
	"strconv"
	"strings"

	"eucli-box/pkg/types"
)

// VersionFormality 表示版本号的段数身份。
type VersionFormality string

const (
	FormalityFormal      VersionFormality = "formal"
	FormalityDevelopment VersionFormality = "development"
)

// semanticVersion 是主.次.补( [.开发序号] )版本。
type semanticVersion struct {
	major    int
	minor    int
	patch    int
	build    int
	hasBuild bool
}

func ValidateVersion(value string) error {
	_, err := parseVersion(value)
	return err
}

// ValidateFormalVersion 校验三段式正式版本。
func ValidateFormalVersion(value string) error {
	version, err := parseVersion(value)
	if err != nil {
		return err
	}
	if version.hasBuild {
		return fmt.Errorf("正式版本必须使用三段式，例如 0.1.0")
	}
	return nil
}

// Formality 返回版本身份：formal（三段）或 development（四段）。
func Formality(value string) (VersionFormality, error) {
	version, err := parseVersion(value)
	if err != nil {
		return "", err
	}
	if version.hasBuild {
		return FormalityDevelopment, nil
	}
	return FormalityFormal, nil
}

// ValidateDevelopmentVersion 校验四段开发版本的前三段与源码正式基线完全一致。
func ValidateDevelopmentVersion(baseVersion string, developmentVersion string) error {
	base, err := parseVersion(baseVersion)
	if err != nil {
		return fmt.Errorf("源码正式基线无效：%w", err)
	}
	if base.hasBuild {
		return fmt.Errorf("源码正式基线必须是三段式，例如 0.1.0")
	}
	development, err := parseVersion(developmentVersion)
	if err != nil {
		return err
	}
	if !development.hasBuild {
		return fmt.Errorf("开发版本必须使用四段式，例如 0.1.0.1")
	}
	if development.major != base.major || development.minor != base.minor || development.patch != base.patch {
		return fmt.Errorf("开发版本 %s 的基线必须与源码正式版本 %s 一致", strings.TrimSpace(developmentVersion), strings.TrimSpace(baseVersion))
	}
	return nil
}

func CompareVersions(left string, right string) (int, error) {
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

func ValidateEucliBoxCompatibility(compatibility types.EucliBoxCompatibility) error {
	minimum, err := parseVersion(compatibility.MinimumVersion)
	if err != nil {
		return fmt.Errorf("最低适用版本无效：%w", err)
	}
	maximum, err := parseVersion(compatibility.MaximumVersionExclusive)
	if err != nil {
		return fmt.Errorf("最高适用边界无效：%w", err)
	}
	if compare(minimum, maximum) >= 0 {
		return fmt.Errorf("最低适用版本必须低于最高适用边界")
	}
	return nil
}

func AssessEucliBoxCompatibility(artifactVersion string, currentEucliBoxVersion string, compatibility types.EucliBoxCompatibility) types.CompatibilityStatus {
	status := types.CompatibilityStatus{
		CurrentEucliBoxVersion:        strings.TrimSpace(currentEucliBoxVersion),
		RequiredEucliBoxCompatibility: compatibility,
	}
	if err := ValidateVersion(artifactVersion); err != nil {
		status.Reason = "发布物版本无效：" + err.Error()
		return status
	}
	if err := ValidateEucliBoxCompatibility(compatibility); err != nil {
		status.Reason = "适用范围无效：" + err.Error()
		return status
	}
	current, err := parseVersion(currentEucliBoxVersion)
	if err != nil {
		status.Reason = "当前 eucli-box 版本无效：" + err.Error()
		return status
	}
	minimum, _ := parseVersion(compatibility.MinimumVersion)
	maximum, _ := parseVersion(compatibility.MaximumVersionExclusive)
	if compare(current, minimum) < 0 || compare(current, maximum) >= 0 {
		status.Reason = fmt.Sprintf("当前 eucli-box 版本 %s 不在所需范围 [%s, %s) 内", currentEucliBoxVersion, compatibility.MinimumVersion, compatibility.MaximumVersionExclusive)
		return status
	}
	status.Compatible = true
	return status
}

func FormatEucliBoxCompatibility(compatibility types.EucliBoxCompatibility) string {
	return fmt.Sprintf("[%s, %s)", compatibility.MinimumVersion, compatibility.MaximumVersionExclusive)
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
