package release

import (
	"fmt"
	"strconv"
	"strings"

	"eucli-box/pkg/types"
)

type semanticVersion struct {
	major int
	minor int
	patch int
}

func ValidateVersion(value string) error {
	_, err := parseVersion(value)
	return err
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
	if len(parts) != 3 {
		return semanticVersion{}, fmt.Errorf("版本必须使用三段正式版本，例如 0.1.0")
	}
	values := [3]int{}
	for index, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return semanticVersion{}, fmt.Errorf("版本必须使用三段正式版本，例如 0.1.0")
		}
		parsed, err := strconv.Atoi(part)
		if err != nil || parsed < 0 {
			return semanticVersion{}, fmt.Errorf("版本必须使用三段正式版本，例如 0.1.0")
		}
		values[index] = parsed
	}
	return semanticVersion{major: values[0], minor: values[1], patch: values[2]}, nil
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
	return 0
}
