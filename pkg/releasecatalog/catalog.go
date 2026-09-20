package releasecatalog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

// releaseKinds 是正式发布物与官方来源共同允许的类别集合，发布物身份规则的唯一事实源。
var releaseKinds = map[string]struct{}{
	types.ReleaseArtifactKindTool:   {},
	types.ReleaseArtifactKindPlugin: {},
}

// ValidateArtifactIdentity 校验发布物身份：类别必须为 AI 工具或系统插件，ID 必须为合法单段标识。
// 一切接受发布物身份作为输入的开发工具统一复用此处，身份规则只维护一份。
func ValidateArtifactIdentity(identity types.ReleaseArtifactIdentity) error {
	if _, ok := releaseKinds[strings.TrimSpace(identity.Kind)]; !ok {
		return fmt.Errorf("发布物类别 %q 无效", identity.Kind)
	}
	if !validID(identity.ID) {
		return fmt.Errorf("发布物 ID %q 无效", identity.ID)
	}
	return nil
}

func Target(identity types.ReleaseArtifactIdentity) string {
	return identity.Kind + ":" + identity.ID
}

func TagName(identity types.ReleaseArtifactIdentity, version string) (string, error) {
	if err := release.ValidateVersion(version); err != nil {
		return "", err
	}
	switch identity.Kind {
	case types.ReleaseArtifactKindTool, types.ReleaseArtifactKindPlugin:
		if !validID(identity.ID) {
			return "", fmt.Errorf("发布物 ID 无效")
		}
		return identity.ID + "/v" + version, nil
	default:
		return "", fmt.Errorf("发布物类别无效")
	}
}

func ArchiveName(identity types.ReleaseArtifactIdentity, version string) (string, error) {
	if _, err := TagName(identity, version); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s_%s_%s.zip", identity.Kind, identity.ID, version, types.ReleasePlatformWindowsX64), nil
}

type repositoryParts struct {
	repository string
	owner      string
	name       string
}

func normalizeRepository(value string) (repositoryParts, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return repositoryParts{}, err
	}
	if parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, "github.com") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return repositoryParts{}, fmt.Errorf("必须是固定的 GitHub HTTPS 仓库地址")
	}
	cleanPath := strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/")
	parts := strings.Split(cleanPath, "/")
	if len(parts) != 2 || !validID(parts[0]) || !validID(parts[1]) {
		return repositoryParts{}, fmt.Errorf("必须精确指向一个 GitHub 仓库")
	}
	repository := "https://github.com/" + parts[0] + "/" + parts[1]
	return repositoryParts{repository: repository, owner: parts[0], name: parts[1]}, nil
}

func validID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || path.Base(value) != value {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("存在多余内容")
		}
		return err
	}
	return nil
}
