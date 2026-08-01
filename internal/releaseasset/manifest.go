package releaseasset

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"eucli-box/pkg/types"
)

type VendorManifest struct {
	SchemaVersion int                       `json:"schemaVersion"`
	Name          string                    `json:"name"`
	Version       string                    `json:"version"`
	Platform      string                    `json:"platform"`
	Source        string                    `json:"source"`
	Sources       []string                  `json:"sources"`
	Inputs        []InputFile               `json:"inputs,omitempty"`
	Files         []types.ReleaseFileRecord `json:"files"`
	TreeSHA256    string                    `json:"treeSha256"`
}

func inspectDirectory(root string, recipe Recipe) (VendorManifest, error) {
	root, err := existingDirectory(root)
	if err != nil {
		return VendorManifest{}, err
	}
	manifest, err := readManifest(root)
	if err != nil {
		return VendorManifest{}, err
	}
	if manifest.SchemaVersion != manifestSchemaVersion || manifest.Name != recipe.Name || manifest.Version != recipe.Version || manifest.Platform != types.ReleasePlatformWindowsX64 || manifest.Source != recipe.Source {
		return VendorManifest{}, fmt.Errorf("外部随包内容 %s 的身份、版本、平台或来源与固定配方不一致", recipe.Name)
	}
	if !reflect.DeepEqual(manifest.Sources, recipe.Sources) || !reflect.DeepEqual(manifest.Inputs, recipe.Inputs) {
		return VendorManifest{}, fmt.Errorf("外部随包内容 %s 的来源输入与固定配方不一致", recipe.Name)
	}
	if len(manifest.Files) == 0 || !validSHA256(manifest.TreeSHA256) {
		return VendorManifest{}, fmt.Errorf("外部随包内容 %s 缺少完整文件清单", recipe.Name)
	}
	actual, err := recordsForDirectory(root, VendorManifestName)
	if err != nil {
		return VendorManifest{}, err
	}
	if !reflect.DeepEqual(actual, manifest.Files) {
		return VendorManifest{}, fmt.Errorf("外部随包内容 %s 的逐文件完整性与清单不一致", recipe.Name)
	}
	if treeSHA256(actual) != manifest.TreeSHA256 {
		return VendorManifest{}, fmt.Errorf("外部随包内容 %s 的目录完整性与清单不一致", recipe.Name)
	}
	for _, required := range recipe.RequiredFiles {
		record, ok := findRecord(actual, filepath.ToSlash(required.Path))
		if !ok || record.Size != required.Size || required.SHA256 != "" && !strings.EqualFold(record.SHA256, required.SHA256) {
			return VendorManifest{}, fmt.Errorf("外部随包内容 %s 的必需文件不一致：%s", recipe.Name, required.Path)
		}
	}
	return manifest, nil
}

func readManifest(root string) (VendorManifest, error) {
	payload, err := os.ReadFile(filepath.Join(root, VendorManifestName))
	if err != nil {
		return VendorManifest{}, fmt.Errorf("外部随包内容缺少 %s：%w", VendorManifestName, err)
	}
	var manifest VendorManifest
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return VendorManifest{}, fmt.Errorf("外部随包内容清单无效：%w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return VendorManifest{}, fmt.Errorf("外部随包内容清单无效：%w", err)
	}
	return manifest, nil
}

func writeManifest(root string, recipe Recipe) (VendorManifest, error) {
	if err := os.Remove(filepath.Join(root, VendorManifestName)); err != nil && !os.IsNotExist(err) {
		return VendorManifest{}, err
	}
	files, err := recordsForDirectory(root, VendorManifestName)
	if err != nil {
		return VendorManifest{}, err
	}
	manifest := VendorManifest{
		SchemaVersion: manifestSchemaVersion,
		Name:          recipe.Name,
		Version:       recipe.Version,
		Platform:      types.ReleasePlatformWindowsX64,
		Source:        recipe.Source,
		Sources:       append([]string(nil), recipe.Sources...),
		Inputs:        append([]InputFile(nil), recipe.Inputs...),
		Files:         files,
		TreeSHA256:    treeSHA256(files),
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return VendorManifest{}, err
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(filepath.Join(root, VendorManifestName), payload, 0o644); err != nil {
		return VendorManifest{}, err
	}
	return manifest, nil
}

func recordsForDirectory(root string, excluded ...string) ([]types.ReleaseFileRecord, error) {
	excludedSet := map[string]struct{}{}
	for _, name := range excluded {
		excludedSet[filepath.ToSlash(name)] = struct{}{}
	}
	records := make([]types.ReleaseFileRecord, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("外部随包内容不能包含符号链接：%s", path)
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if _, skip := excludedSet[name]; skip {
			return nil
		}
		record, err := recordForFile(path, name)
		if err != nil {
			return err
		}
		records = append(records, record)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i int, j int) bool { return records[i].Name < records[j].Name })
	return records, nil
}

func recordForFile(path string, name string) (types.ReleaseFileRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return types.ReleaseFileRecord{}, err
	}
	defer file.Close()
	digest := sha256.New()
	size, err := io.Copy(digest, file)
	if err != nil {
		return types.ReleaseFileRecord{}, err
	}
	return types.ReleaseFileRecord{Name: filepath.ToSlash(name), Size: size, SHA256: hex.EncodeToString(digest.Sum(nil))}, nil
}

func treeSHA256(files []types.ReleaseFileRecord) string {
	digest := sha256.New()
	for _, file := range files {
		_, _ = fmt.Fprintf(digest, "%s\x00%d\x00%s\n", file.Name, file.Size, strings.ToLower(file.SHA256))
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func findRecord(files []types.ReleaseFileRecord, name string) (types.ReleaseFileRecord, bool) {
	index := sort.Search(len(files), func(index int) bool { return files[index].Name >= name })
	if index < len(files) && files[index].Name == name {
		return files[index], true
	}
	return types.ReleaseFileRecord{}, false
}

func existingDirectory(value string) (string, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(value))
	if err != nil || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("外部随包内容目录无效")
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("外部随包内容必须是目录")
	}
	return filepath.Clean(absolute), nil
}
