// Package artifactcatalog 维护正式发布物名册：官方允许正式发布的 AI 工具与系统插件清单。
// 它只服务发布工具链（构建、发布、验证的目标与白名单），业务端不引用本包，
// 因此名册的增删改不需要重新发布业务端。
package artifactcatalog

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
)

//go:embed catalog.json
var catalogPayload []byte

// Catalog 是正式发布物名册。
type Catalog struct {
	SchemaVersion int                             `json:"schemaVersion"`
	Platform      string                          `json:"platform"`
	Artifacts     []types.ReleaseArtifactIdentity `json:"artifacts"`
}

// Load 读取并校验正式发布物名册。
func Load() (Catalog, error) {
	var catalog Catalog
	decoder := json.NewDecoder(bytes.NewReader(catalogPayload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil {
		return Catalog{}, fmt.Errorf("读取正式发行名册失败：%w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Catalog{}, fmt.Errorf("读取正式发行名册失败：存在多余内容")
		}
		return Catalog{}, fmt.Errorf("读取正式发行名册失败：%w", err)
	}
	if err := Validate(catalog); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

// Validate 校验名册结构：平台、发布物身份合法性与去重。
func Validate(catalog Catalog) error {
	if catalog.SchemaVersion != 1 {
		return fmt.Errorf("正式发行名册 schemaVersion 必须为 1")
	}
	if catalog.Platform != types.ReleasePlatformWindowsX64 {
		return fmt.Errorf("正式发行名册平台必须为 %s", types.ReleasePlatformWindowsX64)
	}
	identities := map[string]struct{}{}
	for _, artifact := range catalog.Artifacts {
		artifact.Kind = strings.TrimSpace(artifact.Kind)
		artifact.ID = strings.TrimSpace(artifact.ID)
		if err := releasecatalog.ValidateArtifactIdentity(artifact); err != nil {
			return fmt.Errorf("正式发行名册包含无效发布物：%w", err)
		}
		key := artifact.Kind + ":" + artifact.ID
		if _, exists := identities[key]; exists {
			return fmt.Errorf("正式发行名册包含重复发布物 %s", key)
		}
		identities[key] = struct{}{}
	}
	return nil
}

// Contains 判断发布物是否在正式发行名册内。
func (c Catalog) Contains(identity types.ReleaseArtifactIdentity) bool {
	for _, artifact := range c.Artifacts {
		if artifact.Kind == identity.Kind && artifact.ID == identity.ID {
			return true
		}
	}
	return false
}

// SortedArtifacts 返回按类别与 ID 稳定排序的全部名册发布物。
func (c Catalog) SortedArtifacts() []types.ReleaseArtifactIdentity {
	artifacts := append([]types.ReleaseArtifactIdentity(nil), c.Artifacts...)
	sort.Slice(artifacts, func(i int, j int) bool {
		if artifacts[i].Kind != artifacts[j].Kind {
			return artifacts[i].Kind < artifacts[j].Kind
		}
		return artifacts[i].ID < artifacts[j].ID
	})
	return artifacts
}

// ResolveTarget 解析正式发布目标并校验名册收录。
func (c Catalog) ResolveTarget(target string) (types.ReleaseArtifactIdentity, error) {
	target = strings.TrimSpace(target)
	kind, id, ok := strings.Cut(target, ":")
	if !ok {
		return types.ReleaseArtifactIdentity{}, invalidTargetError()
	}
	identity := types.ReleaseArtifactIdentity{Kind: strings.TrimSpace(kind), ID: strings.TrimSpace(id)}
	if !c.Contains(identity) {
		return types.ReleaseArtifactIdentity{}, fmt.Errorf("%s 不在正式发布物白名单中", target)
	}
	return identity, nil
}

func invalidTargetError() error {
	return fmt.Errorf("正式发布目标必须是 tool:<id> 或 plugin:<id>")
}
