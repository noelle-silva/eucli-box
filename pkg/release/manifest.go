package release

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"eucli-box/pkg/types"
)

const ReleaseManifestSchemaVersion = 1

func DecodeReleaseManifest(payload []byte) (types.ReleaseManifest, error) {
	var manifest types.ReleaseManifest
	if err := decodeStrictJSON(payload, &manifest); err != nil {
		return types.ReleaseManifest{}, fmt.Errorf("发行清单无效：%w", err)
	}
	if err := ValidateReleaseManifest(manifest); err != nil {
		return types.ReleaseManifest{}, err
	}
	return manifest, nil
}

func DecodeReleaseProductRecord(payload []byte) (types.ReleaseProductRecord, error) {
	var record types.ReleaseProductRecord
	if err := decodeStrictJSON(payload, &record); err != nil {
		return types.ReleaseProductRecord{}, fmt.Errorf("成品身份资料无效：%w", err)
	}
	if err := ValidateReleaseProductRecord(record); err != nil {
		return types.ReleaseProductRecord{}, err
	}
	return record, nil
}

func ValidateReleaseManifest(manifest types.ReleaseManifest) error {
	if manifest.SchemaVersion != ReleaseManifestSchemaVersion {
		return fmt.Errorf("发行清单 schemaVersion 必须为 %d", ReleaseManifestSchemaVersion)
	}
	if err := validateIdentity(manifest.Artifact); err != nil {
		return err
	}
	if err := ValidateVersion(manifest.Version); err != nil {
		return fmt.Errorf("发行版本无效：%w", err)
	}
	if manifest.Platform != types.ReleasePlatformWindowsX64 {
		return fmt.Errorf("发行平台必须为 %s", types.ReleasePlatformWindowsX64)
	}
	if err := validateOfficialSource(manifest.OfficialSource); err != nil {
		return err
	}
	if strings.TrimSpace(manifest.TagName) == "" {
		return fmt.Errorf("发行标签不能为空")
	}
	if err := validateReleaseSpecificFacts(manifest.Artifact, manifest.Compatibility, manifest.DataVersion); err != nil {
		return err
	}
	if err := validateExternalAssets(manifest.ExternalAssets); err != nil {
		return err
	}
	if err := validateSourceRecord(manifest.Source); err != nil {
		return err
	}
	if err := validateFileRecord(manifest.Archive); err != nil {
		return fmt.Errorf("压缩包资料无效：%w", err)
	}
	if !strings.HasSuffix(strings.ToLower(manifest.Archive.Name), ".zip") {
		return fmt.Errorf("正式成品必须是 ZIP 压缩包")
	}
	if len(manifest.Files) == 0 {
		return fmt.Errorf("发行清单必须列出包内文件")
	}
	seen := map[string]struct{}{}
	previous := ""
	for _, file := range manifest.Files {
		if err := validateFileRecord(file); err != nil {
			return fmt.Errorf("包内文件资料无效：%w", err)
		}
		if _, exists := seen[file.Name]; exists {
			return fmt.Errorf("发行清单包含重复文件 %s", file.Name)
		}
		if previous != "" && file.Name < previous {
			return fmt.Errorf("发行清单中的包内文件必须按路径排序")
		}
		seen[file.Name] = struct{}{}
		previous = file.Name
	}
	if _, ok := seen["release-product.json"]; !ok {
		return fmt.Errorf("正式成品缺少 release-product.json")
	}
	return nil
}

func ValidateReleaseProductRecord(record types.ReleaseProductRecord) error {
	if record.SchemaVersion != ReleaseManifestSchemaVersion {
		return fmt.Errorf("成品身份资料 schemaVersion 必须为 %d", ReleaseManifestSchemaVersion)
	}
	if err := validateIdentity(record.Artifact); err != nil {
		return err
	}
	if err := ValidateVersion(record.Version); err != nil {
		return fmt.Errorf("成品版本无效：%w", err)
	}
	if record.Platform != types.ReleasePlatformWindowsX64 {
		return fmt.Errorf("成品平台必须为 %s", types.ReleasePlatformWindowsX64)
	}
	if err := validateOfficialSource(record.OfficialSource); err != nil {
		return err
	}
	if err := validateReleaseSpecificFacts(record.Artifact, record.Compatibility, record.DataVersion); err != nil {
		return err
	}
	if err := validateExternalAssets(record.ExternalAssets); err != nil {
		return err
	}
	if err := validateSourceRecord(record.Source); err != nil {
		return err
	}
	return nil
}

func ValidateManifestIdentity(manifest types.ReleaseManifest, expected types.ReleaseArtifactIdentity, expectedTag string, expectedRepository string) error {
	if manifest.Artifact != expected {
		return fmt.Errorf("发行清单身份与目标发布物不一致")
	}
	if manifest.TagName != strings.TrimSpace(expectedTag) {
		return fmt.Errorf("发行清单标签与正式发行不一致")
	}
	if !strings.EqualFold(strings.TrimSuffix(manifest.OfficialSource, "/"), strings.TrimSuffix(strings.TrimSpace(expectedRepository), "/")) {
		return fmt.Errorf("发行清单来源不是目标发布物的固定官方来源")
	}
	return nil
}

func ValidateArchiveDigest(manifest types.ReleaseManifest, payload []byte) error {
	if int64(len(payload)) != manifest.Archive.Size {
		return fmt.Errorf("压缩包大小与发行清单不一致")
	}
	if SHA256(payload) != manifest.Archive.SHA256 {
		return fmt.Errorf("压缩包完整性与发行清单不一致")
	}
	return nil
}

func SHA256(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func SortedFileRecords(records []types.ReleaseFileRecord) []types.ReleaseFileRecord {
	result := append([]types.ReleaseFileRecord(nil), records...)
	sort.Slice(result, func(i int, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func validateReleaseSpecificFacts(identity types.ReleaseArtifactIdentity, compatibility *types.EucliBoxCompatibility, dataVersion string) error {
	switch identity.Kind {
	case types.ReleaseArtifactKindBox:
		if compatibility != nil {
			return fmt.Errorf("业务端发行不能声明对自身的适用范围")
		}
		if err := ValidateVersion(dataVersion); err != nil {
			return fmt.Errorf("业务端目标数据版本无效：%w", err)
		}
	case types.ReleaseArtifactKindTool, types.ReleaseArtifactKindPlugin:
		if compatibility == nil {
			return fmt.Errorf("工具和插件必须声明业务端适用范围")
		}
		if err := ValidateEucliBoxCompatibility(*compatibility); err != nil {
			return fmt.Errorf("业务端适用范围无效：%w", err)
		}
		if strings.TrimSpace(dataVersion) != "" {
			return fmt.Errorf("工具和插件不能声明业务端目标数据版本")
		}
	default:
		return fmt.Errorf("发布物类别无效")
	}
	return nil
}

func validateIdentity(identity types.ReleaseArtifactIdentity) error {
	identity.Kind = strings.TrimSpace(identity.Kind)
	identity.ID = strings.TrimSpace(identity.ID)
	if identity.ID == "" {
		return fmt.Errorf("发布物 ID 不能为空")
	}
	switch identity.Kind {
	case types.ReleaseArtifactKindBox:
		if identity.ID != types.ReleaseArtifactKindBox {
			return fmt.Errorf("业务端发布物 ID 必须为 %s", types.ReleaseArtifactKindBox)
		}
	case types.ReleaseArtifactKindTool, types.ReleaseArtifactKindPlugin:
	default:
		return fmt.Errorf("发布物类别无效")
	}
	if strings.ContainsAny(identity.ID, `/\\`) || identity.ID == "." || identity.ID == ".." {
		return fmt.Errorf("发布物 ID 无效")
	}
	return nil
}

func validateSourceRecord(source types.ReleaseSourceRecord) error {
	if !strings.HasPrefix(strings.TrimSpace(source.Repository), "https://github.com/") {
		return fmt.Errorf("源码来源必须指向固定 GitHub 仓库")
	}
	commit := strings.TrimSpace(source.Commit)
	if len(commit) != 40 {
		return fmt.Errorf("源码状态必须使用完整 Git 提交编号")
	}
	for _, char := range commit {
		if char >= '0' && char <= '9' || char >= 'a' && char <= 'f' {
			continue
		}
		return fmt.Errorf("源码提交编号无效")
	}
	return nil
}

func validateOfficialSource(source string) error {
	source = strings.TrimSuffix(strings.TrimSpace(source), "/")
	if !strings.HasPrefix(source, "https://github.com/") {
		return fmt.Errorf("官方来源必须指向固定 GitHub 仓库")
	}
	parts := strings.Split(strings.TrimPrefix(source, "https://github.com/"), "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return fmt.Errorf("官方来源必须精确指向一个 GitHub 仓库")
	}
	return nil
}

func validateExternalAssets(assets []types.ReleaseExternalAsset) error {
	seen := map[string]struct{}{}
	previous := ""
	for _, asset := range assets {
		asset.Name = strings.TrimSpace(asset.Name)
		asset.Source = strings.TrimSpace(asset.Source)
		asset.Version = strings.TrimSpace(asset.Version)
		asset.PackagePath = strings.TrimSpace(asset.PackagePath)
		if asset.Name == "" || strings.ContainsAny(asset.Name, `/\\`) {
			return fmt.Errorf("外部附带内容名称无效")
		}
		if previous != "" && asset.Name < previous {
			return fmt.Errorf("外部附带内容必须按名称排序")
		}
		if _, exists := seen[asset.Name]; exists {
			return fmt.Errorf("外部附带内容 %s 重复", asset.Name)
		}
		seen[asset.Name] = struct{}{}
		previous = asset.Name
		if !strings.HasPrefix(asset.Source, "https://") {
			return fmt.Errorf("外部附带内容 %s 必须声明 HTTPS 官方来源", asset.Name)
		}
		if asset.Version == "" || asset.FileCount <= 0 {
			return fmt.Errorf("外部附带内容 %s 的版本或文件数量无效", asset.Name)
		}
		if err := validatePackagePath(asset.PackagePath); err != nil {
			return fmt.Errorf("外部附带内容 %s 的成品位置无效：%w", asset.Name, err)
		}
		if len(asset.TreeSHA256) != sha256.Size*2 {
			return fmt.Errorf("外部附带内容 %s 缺少有效目录完整性", asset.Name)
		}
		if _, err := hex.DecodeString(asset.TreeSHA256); err != nil {
			return fmt.Errorf("外部附带内容 %s 的目录完整性无效", asset.Name)
		}
	}
	return nil
}

func validatePackagePath(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") {
		return fmt.Errorf("必须是非空相对目录")
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("必须是规范的相对目录")
		}
	}
	return nil
}

func validateFileRecord(file types.ReleaseFileRecord) error {
	name := strings.TrimSpace(file.Name)
	if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || strings.Contains("/"+name+"/", "/../") || strings.Contains("/"+name+"/", "/./") {
		return fmt.Errorf("文件路径 %q 无效", file.Name)
	}
	if file.Size < 0 {
		return fmt.Errorf("文件 %s 大小无效", file.Name)
	}
	if len(file.SHA256) != sha256.Size*2 {
		return fmt.Errorf("文件 %s 缺少有效 SHA-256", file.Name)
	}
	if _, err := hex.DecodeString(file.SHA256); err != nil {
		return fmt.Errorf("文件 %s 的 SHA-256 无效", file.Name)
	}
	return nil
}

func decodeStrictJSON(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("存在多余内容")
		}
		return err
	}
	return nil
}
