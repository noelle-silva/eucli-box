package releaseasset

import (
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
		{artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "shell_command"}, want: []string{"git-bash-root", "nushell-root", "powershell-root"}},
		{artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "sci_calculator"}, want: []string{"sci-calculator-python-runtime"}},
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
