package releaseasset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"eucli-box/pkg/types"
)

func TestCatalogDefinesFixedAssetsForReleaseArtifacts(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		artifact types.ReleaseArtifactIdentity
		want     []string
	}{
		{artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "everything"}, want: []string{"everything-root"}},
		{artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "shell_command"}, want: []string{"command-analyzer-root", "git-bash-root", "nushell-root", "powershell-root"}},
	}
	for _, testCase := range cases {
		recipes := catalog.RecipesForArtifact(testCase.artifact)
		if len(recipes) != len(testCase.want) {
			t.Fatalf("%s:%s recipes = %#v", testCase.artifact.Kind, testCase.artifact.ID, recipes)
		}
		for index, want := range testCase.want {
			if recipes[index].Name != want {
				t.Fatalf("%s:%s recipes = %#v", testCase.artifact.Kind, testCase.artifact.ID, recipes)
			}
		}
	}
	if recipes := catalog.RecipesForArtifact(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindBox, ID: types.ReleaseArtifactKindBox}); len(recipes) != 0 {
		t.Fatalf("eucli-box must not bundle tool assets: %#v", recipes)
	}
}

// TestToolpackAssetRootsAreCoveredByRecipes 锁住"工具打包声明 ↔ 发行资产准备"两个事实源：
// 任何工具 toolpack.json 声明的资产根都必须有对应的发行配方，否则打包出的成品必然残缺。
func TestToolpackAssetRootsAreCoveredByRecipes(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	repositoryRoot := filepath.Join("..", "..", "..")
	if _, err := os.Stat(filepath.Join(repositoryRoot, "go.mod")); err != nil {
		t.Fatalf("仓库根目录不可达：%v", err)
	}
	entries, err := os.ReadDir(filepath.Join(repositoryRoot, "tools"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		payload, err := os.ReadFile(filepath.Join(repositoryRoot, "tools", entry.Name(), "toolpack.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var spec struct {
			AssetRoots []struct {
				Name string `json:"name"`
			} `json:"assetRoots"`
		}
		if err := json.Unmarshal(payload, &spec); err != nil {
			t.Fatalf("读取 %s 的打包声明失败：%v", entry.Name(), err)
		}
		artifact := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: entry.Name()}
		prepared := map[string]struct{}{}
		for _, recipe := range catalog.RecipesForArtifact(artifact) {
			prepared[recipe.Name] = struct{}{}
		}
		for _, asset := range spec.AssetRoots {
			if _, ok := prepared[asset.Name]; !ok {
				t.Errorf("工具 %s 声明了资产根 %q，但发行侧没有对应配方", entry.Name(), asset.Name)
			}
		}
	}
}

func TestInspectRejectsChangedContent(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := catalog.Recipe("everything-root")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := writeTestAssetTree(root, recipe); err != nil {
		t.Fatal(err)
	}
	if _, err := writeManifest(root, recipe); err != nil {
		t.Fatal(err)
	}
	if err := appendFile(root, recipe.RequiredFiles[0].Path); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectDirectory(root, recipe); err == nil {
		t.Fatal("changed asset content must be rejected")
	}
}
