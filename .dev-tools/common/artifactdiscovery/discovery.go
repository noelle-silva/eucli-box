// Package artifactdiscovery 从仓库事实里找出可发布的正式发布物：
// 工具看 tools/<id>/tool.json，插件看 system-plugins/<id>/manifest.json。
// 能发布什么由仓库里实际存在的东西决定，不再维护一份手工名册。
package artifactdiscovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
)

const (
	toolsDirectory   = "tools"
	pluginsDirectory = "system-plugins"
	toolManifestFile = "tool.json"
	pluginManifest   = "manifest.json"
	// 模板目录不是发布物：以下划线开头的一律跳过。
	templatePrefix = "_"
)

// ParseTarget 解析发布目标：只校验 kind:id 形态与身份合法性，
// 不再查询任何名单——仓库里存在什么就能发布什么。
func ParseTarget(target string) (types.ReleaseArtifactIdentity, error) {
	kind, id, ok := strings.Cut(strings.TrimSpace(target), ":")
	identity := types.ReleaseArtifactIdentity{Kind: strings.TrimSpace(kind), ID: strings.TrimSpace(id)}
	if !ok || identity.Kind == "" || identity.ID == "" {
		return types.ReleaseArtifactIdentity{}, fmt.Errorf("发布目标必须是 tool:<id> 或 plugin:<id>")
	}
	if err := releasecatalog.ValidateArtifactIdentity(identity); err != nil {
		return types.ReleaseArtifactIdentity{}, err
	}
	return identity, nil
}

// Discover 扫描仓库，返回按类别与标识稳定排序的全部发布物。
func Discover(root string) ([]types.ReleaseArtifactIdentity, error) {
	tools, err := discoverIn(filepath.Join(root, toolsDirectory), types.ReleaseArtifactKindTool, toolManifestFile)
	if err != nil {
		return nil, err
	}
	plugins, err := discoverIn(filepath.Join(root, pluginsDirectory), types.ReleaseArtifactKindPlugin, pluginManifest)
	if err != nil {
		return nil, err
	}
	identities := append(tools, plugins...)
	sort.Slice(identities, func(i int, j int) bool {
		if identities[i].Kind != identities[j].Kind {
			return identities[i].Kind < identities[j].Kind
		}
		return identities[i].ID < identities[j].ID
	})
	return identities, nil
}

// discoverIn 扫描一类目录：每个子目录里的清单给出自己的标识。
func discoverIn(directory string, kind string, manifestName string) ([]types.ReleaseArtifactIdentity, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取目录失败（%s）：%w", directory, err)
	}
	identities := make([]types.ReleaseArtifactIdentity, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), templatePrefix) {
			continue
		}
		manifestPath := filepath.Join(directory, entry.Name(), manifestName)
		id, readErr := readManifestID(manifestPath)
		if readErr != nil {
			return nil, readErr
		}
		identity := types.ReleaseArtifactIdentity{Kind: kind, ID: id}
		if err := releasecatalog.ValidateArtifactIdentity(identity); err != nil {
			return nil, fmt.Errorf("发布物清单身份无效（%s）：%w", manifestPath, err)
		}
		identities = append(identities, identity)
	}
	return identities, nil
}

// readManifestID 只取清单里的 id：清单本身由各自的打包规则校验，
// 这里只回答「这个目录自称是哪个发布物」。
func readManifestID(path string) (string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取发布物清单失败（%s）：%w", path, err)
	}
	var manifest struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return "", fmt.Errorf("解析发布物清单失败（%s）：%w", path, err)
	}
	id := strings.TrimSpace(manifest.ID)
	if id == "" {
		return "", fmt.Errorf("发布物清单缺少 id（%s）", path)
	}
	return id, nil
}
