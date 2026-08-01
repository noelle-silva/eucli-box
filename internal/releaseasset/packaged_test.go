package releaseasset

import (
	"os"
	"path/filepath"
	"testing"

	"eucli-box/pkg/types"
)

func TestValidatePackagedAssetsAcceptsFixedAssetAtPackagedRoot(t *testing.T) {
	repositoryRoot := findRepositoryRootForTest(t)
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := catalog.Recipe("everything-root")
	if err != nil {
		t.Fatal(err)
	}
	packageRoot := t.TempDir()
	assetRoot := filepath.Join(packageRoot, "providers", "everything")
	sourceRoot := filepath.Join(repositoryRoot, "tools", "everything", "providers", "everything")
	if err := copyTestDirectory(sourceRoot, assetRoot); err != nil {
		t.Fatalf("copy fixed asset: %v", err)
	}
	manifest, err := readManifest(assetRoot)
	if err != nil {
		t.Fatalf("read copied manifest: %v", err)
	}
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "everything"}
	assets, err := BindPackagedAssets(packageRoot, identity, []types.ReleaseExternalAsset{externalAssetRecord(recipe, manifest)})
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].PackagePath != "providers/everything" {
		t.Fatalf("packaged assets = %#v", assets)
	}
	roots, err := ValidatePackagedAssets(packageRoot, identity, assets)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || roots[0] != "providers/everything" {
		t.Fatalf("validated roots = %#v", roots)
	}
}

func copyTestDirectory(source string, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		return os.WriteFile(destination, payload, 0o644)
	})
}

func findRepositoryRootForTest(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	current, err := filepath.Abs(workingDirectory)
	if err != nil {
		t.Fatalf("resolve working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			t.Fatalf("repository root not found from %s", workingDirectory)
		}
		current = parent
	}
}

func TestValidatePackagedAssetsRejectsChangedContent(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := catalog.Recipe("everything-root")
	if err != nil {
		t.Fatal(err)
	}
	packageRoot := t.TempDir()
	assetRoot := filepath.Join(packageRoot, "providers", "everything")
	if err := writeTestAssetTree(assetRoot, recipe); err != nil {
		t.Fatal(err)
	}
	manifest, err := writeManifest(assetRoot, recipe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetRoot, "unexpected.data"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "everything"}
	if _, err := BindPackagedAssets(packageRoot, identity, []types.ReleaseExternalAsset{externalAssetRecord(recipe, manifest)}); err == nil {
		t.Fatal("changed packaged content was accepted")
	}
}

func TestValidatePackagedAssetsRejectsMovedContent(t *testing.T) {
	repositoryRoot := findRepositoryRootForTest(t)
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := catalog.Recipe("everything-root")
	if err != nil {
		t.Fatal(err)
	}
	packageRoot := t.TempDir()
	assetRoot := filepath.Join(packageRoot, "moved", "everything")
	sourceRoot := filepath.Join(repositoryRoot, "tools", "everything", "providers", "everything")
	if err := copyTestDirectory(sourceRoot, assetRoot); err != nil {
		t.Fatal(err)
	}
	manifest, err := readManifest(assetRoot)
	if err != nil {
		t.Fatal(err)
	}
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "everything"}
	declared := externalAssetRecord(recipe, manifest)
	declared.PackagePath = "providers/everything"
	if _, err := ValidatePackagedAssets(packageRoot, identity, []types.ReleaseExternalAsset{declared}); err == nil {
		t.Fatal("moved packaged content was accepted")
	}
}
