package releaseasset

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"eucli-box/pkg/types"
)

// BindPackagedAssets verifies assembled external assets and records their package locations.
func BindPackagedAssets(packageRoot string, artifact types.ReleaseArtifactIdentity, declared []types.ReleaseExternalAsset) ([]types.ReleaseExternalAsset, error) {
	assets, err := inspectPackagedAssets(packageRoot, artifact, declared, false)
	if err != nil {
		return nil, err
	}
	return assets, nil
}

// ValidatePackagedAssets verifies external assets at the locations recorded in release metadata.
func ValidatePackagedAssets(packageRoot string, artifact types.ReleaseArtifactIdentity, declared []types.ReleaseExternalAsset) ([]string, error) {
	assets, err := inspectPackagedAssets(packageRoot, artifact, declared, true)
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(assets))
	for _, asset := range assets {
		roots = append(roots, asset.PackagePath)
	}
	return roots, nil
}

func inspectPackagedAssets(packageRoot string, artifact types.ReleaseArtifactIdentity, declared []types.ReleaseExternalAsset, requireRecordedPath bool) ([]types.ReleaseExternalAsset, error) {
	packageRoot, err := existingDirectory(packageRoot)
	if err != nil {
		return nil, fmt.Errorf("读取成品目录失败：%w", err)
	}
	catalog, err := LoadCatalog()
	if err != nil {
		return nil, err
	}
	recipes := catalog.RecipesForArtifact(artifact)
	expected := make(map[string]types.ReleaseExternalAsset, len(recipes))
	if len(declared) != len(recipes) {
		return nil, fmt.Errorf("成品声明的外部附带内容与固定配方不一致")
	}
	for index, recipe := range recipes {
		asset := declared[index]
		if asset.Name != recipe.Name || asset.Source != recipe.Source || asset.Version != recipe.Version {
			return nil, fmt.Errorf("成品声明的外部附带内容与固定配方不一致：%s", recipe.Name)
		}
		if requireRecordedPath && !safeRelativePath(asset.PackagePath) {
			return nil, fmt.Errorf("成品外部附带内容 %s 缺少有效成品位置", recipe.Name)
		}
		expected[recipe.Name] = asset
	}

	found := make(map[string]types.ReleaseExternalAsset, len(expected))
	err = filepath.WalkDir(packageRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("成品外部附带内容不能包含符号链接：%s", path)
		}
		if entry.IsDir() || entry.Name() != VendorManifestName {
			return nil
		}
		root := filepath.Dir(path)
		relativeRoot, err := filepath.Rel(packageRoot, root)
		if err != nil {
			return err
		}
		relativeRoot = filepath.ToSlash(relativeRoot)
		if !safeRelativePath(relativeRoot) {
			return fmt.Errorf("成品外部附带内容目录无效：%s", relativeRoot)
		}
		manifest, err := readManifest(root)
		if err != nil {
			return err
		}
		recipe, err := catalog.Recipe(manifest.Name)
		if err != nil {
			return err
		}
		expectedAsset, ok := expected[recipe.Name]
		if !ok {
			return fmt.Errorf("成品包含不属于当前发布物的外部附带内容：%s", recipe.Name)
		}
		if previous, duplicate := found[recipe.Name]; duplicate {
			return fmt.Errorf("成品重复包含外部附带内容 %s：%s、%s", recipe.Name, previous.PackagePath, relativeRoot)
		}
		verified, err := inspectDirectory(root, recipe)
		if err != nil {
			return err
		}
		actual := externalAssetRecord(recipe, verified)
		actual.PackagePath = relativeRoot
		if requireRecordedPath {
			if actual != expectedAsset {
				return fmt.Errorf("成品外部附带内容 %s 与成品身份资料不一致", recipe.Name)
			}
		} else if actual.Name != expectedAsset.Name || actual.Source != expectedAsset.Source || actual.Version != expectedAsset.Version || actual.FileCount != expectedAsset.FileCount || actual.TreeSHA256 != expectedAsset.TreeSHA256 {
			return fmt.Errorf("成品外部附带内容 %s 与准备结果不一致", recipe.Name)
		}
		found[recipe.Name] = actual
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(found) != len(expected) {
		missing := make([]string, 0, len(expected)-len(found))
		for name := range expected {
			if _, ok := found[name]; !ok {
				missing = append(missing, name)
			}
		}
		sort.Strings(missing)
		return nil, fmt.Errorf("成品缺少外部附带内容：%s", strings.Join(missing, "、"))
	}

	assets := make([]types.ReleaseExternalAsset, 0, len(found))
	for _, asset := range found {
		assets = append(assets, asset)
	}
	sort.Slice(assets, func(i int, j int) bool { return assets[i].Name < assets[j].Name })
	for index, asset := range assets {
		for _, other := range assets[index+1:] {
			if strings.HasPrefix(other.PackagePath, asset.PackagePath+"/") {
				return nil, fmt.Errorf("成品外部附带内容目录不能互相嵌套：%s、%s", asset.PackagePath, other.PackagePath)
			}
		}
	}
	return assets, nil
}

func externalAssetRecord(recipe Recipe, manifest VendorManifest) types.ReleaseExternalAsset {
	return types.ReleaseExternalAsset{
		Name:       recipe.Name,
		Source:     recipe.Source,
		Version:    recipe.Version,
		FileCount:  len(manifest.Files),
		TreeSHA256: manifest.TreeSHA256,
	}
}
