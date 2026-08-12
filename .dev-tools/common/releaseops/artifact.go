package releaseops

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

type Kind string

const (
	KindBox    Kind = "eucli-box"
	KindClient Kind = "eucli-studio"
	KindTool   Kind = "tool"
	KindPlugin Kind = "plugin"
)

type Artifact struct {
	Kind          Kind
	ID            string
	Directory     string
	MetadataPath  string
	Version       string
	DataVersion   string
	Compatibility *types.EucliBoxCompatibility
	READMEPath    string
	ChangelogPath string
}

type clientRelease struct {
	Version               string                      `json:"version"`
	EucliBoxCompatibility types.EucliBoxCompatibility `json:"eucliBoxCompatibility"`
}

func Discover(root string) ([]Artifact, error) {
	root, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return nil, fmt.Errorf("确定仓库根目录失败：%w", err)
	}
	artifacts := make([]Artifact, 0)
	for _, target := range []string{"eucli-box", "eucli-studio"} {
		artifact, err := Resolve(root, target)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	for _, group := range []struct {
		kind Kind
		dir  string
	}{
		{kind: KindTool, dir: "tools"},
		{kind: KindPlugin, dir: "system-plugins"},
	} {
		entries, err := os.ReadDir(filepath.Join(root, group.dir))
		if err != nil {
			return nil, fmt.Errorf("读取 %s 目录失败：%w", group.dir, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			artifact, err := Resolve(root, string(group.kind)+":"+entry.Name())
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, artifact)
		}
	}
	sort.Slice(artifacts, func(i int, j int) bool { return artifacts[i].Target() < artifacts[j].Target() })
	return artifacts, nil
}

func Resolve(root string, target string) (Artifact, error) {
	root, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return Artifact{}, fmt.Errorf("确定仓库根目录失败：%w", err)
	}
	target = strings.TrimSpace(target)
	var artifact Artifact
	switch target {
	case "eucli-box":
		artifact = Artifact{Kind: KindBox, ID: "eucli-box", Directory: root, MetadataPath: filepath.Join(root, "internal", "boxrelease", "release.json")}
	case "eucli-studio":
		artifact = Artifact{Kind: KindClient, ID: "eucli-studio", Directory: filepath.Join(root, "clients", "eucli-studio")}
		artifact.MetadataPath = filepath.Join(artifact.Directory, "release.json")
	default:
		kindText, id, ok := strings.Cut(target, ":")
		id = strings.TrimSpace(id)
		if !ok || id == "" || filepath.Base(id) != id {
			return Artifact{}, invalidTargetError()
		}
		switch Kind(kindText) {
		case KindTool:
			artifact = Artifact{Kind: KindTool, ID: id, Directory: filepath.Join(root, "tools", id)}
			artifact.MetadataPath = filepath.Join(artifact.Directory, "tool.json")
		case KindPlugin:
			artifact = Artifact{Kind: KindPlugin, ID: id, Directory: filepath.Join(root, "system-plugins", id)}
			artifact.MetadataPath = filepath.Join(artifact.Directory, "manifest.json")
		default:
			return Artifact{}, invalidTargetError()
		}
	}
	if err := loadMetadata(&artifact); err != nil {
		return Artifact{}, err
	}
	artifact.READMEPath, err = findDocument(artifact.Directory, "README.md")
	if err != nil {
		return Artifact{}, fmt.Errorf("%s：%w", artifact.Target(), err)
	}
	artifact.ChangelogPath, err = findDocument(artifact.Directory, "CHANGELOG.md")
	if err != nil {
		return Artifact{}, fmt.Errorf("%s：%w", artifact.Target(), err)
	}
	return artifact, nil
}

func (a Artifact) Target() string {
	if a.Kind == KindBox || a.Kind == KindClient {
		return string(a.Kind)
	}
	return string(a.Kind) + ":" + a.ID
}

func loadMetadata(artifact *Artifact) error {
	payload, err := os.ReadFile(artifact.MetadataPath)
	if err != nil {
		return fmt.Errorf("读取 %s 发布资料失败：%w", artifact.Target(), err)
	}
	switch artifact.Kind {
	case KindBox:
		var info types.EucliBoxRelease
		if err := decodeStrictJSON(payload, &info); err != nil {
			return metadataError(*artifact, err)
		}
		artifact.Version = strings.TrimSpace(info.Version)
		artifact.DataVersion = strings.TrimSpace(info.DataVersion)
	case KindClient:
		var info clientRelease
		if err := decodeStrictJSON(payload, &info); err != nil {
			return metadataError(*artifact, err)
		}
		artifact.Version = strings.TrimSpace(info.Version)
		artifact.Compatibility = &info.EucliBoxCompatibility
	case KindTool:
		var info types.ToolDefinition
		if err := decodeStrictJSON(payload, &info); err != nil {
			return metadataError(*artifact, err)
		}
		if strings.TrimSpace(info.ID) != artifact.ID {
			return fmt.Errorf("%s 发布资料中的 id 与目录不一致", artifact.Target())
		}
		artifact.Version = strings.TrimSpace(info.Version)
		artifact.Compatibility = &info.EucliBoxCompatibility
	case KindPlugin:
		var info types.SystemPluginManifest
		if err := decodeStrictJSON(payload, &info); err != nil {
			return metadataError(*artifact, err)
		}
		if strings.TrimSpace(info.ID) != artifact.ID {
			return fmt.Errorf("%s 发布资料中的 id 与目录不一致", artifact.Target())
		}
		artifact.Version = strings.TrimSpace(info.Version)
		artifact.Compatibility = &info.EucliBoxCompatibility
	}
	if err := release.ValidateVersion(artifact.Version); err != nil {
		return fmt.Errorf("%s 版本无效：%w", artifact.Target(), err)
	}
	if artifact.Kind == KindBox {
		if err := release.ValidateVersion(artifact.DataVersion); err != nil {
			return fmt.Errorf("%s 数据版本无效：%w", artifact.Target(), err)
		}
	}
	if artifact.Compatibility != nil {
		artifact.Compatibility.MinimumVersion = strings.TrimSpace(artifact.Compatibility.MinimumVersion)
		artifact.Compatibility.MaximumVersionExclusive = strings.TrimSpace(artifact.Compatibility.MaximumVersionExclusive)
		if err := release.ValidateEucliBoxCompatibility(*artifact.Compatibility); err != nil {
			return fmt.Errorf("%s 适用范围无效：%w", artifact.Target(), err)
		}
	}
	return nil
}

func decodeStrictJSON(payload []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("存在多余内容")
	}
	return nil
}

func findDocument(directory string, name string) (string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", fmt.Errorf("读取发布物目录失败：%w", err)
	}
	matches := make([]string, 0, 1)
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(entry.Name(), name) {
			matches = append(matches, filepath.Join(directory, entry.Name()))
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("缺少中文 %s", name)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("存在多份 %s", name)
	}
	return matches[0], nil
}

func invalidTargetError() error {
	return fmt.Errorf("目标必须是 eucli-box、eucli-studio、tool:<id> 或 plugin:<id>")
}

func metadataError(artifact Artifact, err error) error {
	return fmt.Errorf("%s 发布资料无效：%w", artifact.Target(), err)
}
