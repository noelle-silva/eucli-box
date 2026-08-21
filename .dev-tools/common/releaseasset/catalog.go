package releaseasset

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"eucli-box/pkg/types"
)

const (
	manifestSchemaVersion = 1
	VendorManifestName    = "vendor-manifest.json"
)

//go:embed recipes.json
var recipeSource []byte

type Catalog struct {
	SchemaVersion int      `json:"schemaVersion"`
	Platform      string   `json:"platform"`
	Recipes       []Recipe `json:"recipes"`
}

type Recipe struct {
	Name           string             `json:"name"`
	Kind           string             `json:"kind"`
	Version        string             `json:"version"`
	Source         string             `json:"source"`
	Sources        []string           `json:"sources"`
	Artifacts      []string           `json:"artifacts"`
	RepositoryPath string             `json:"repositoryPath,omitempty"`
	Inputs         []InputFile        `json:"inputs,omitempty"`
	RequiredFiles  []PinnedFileRecord `json:"requiredFiles"`
}

type InputFile struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type PinnedFileRecord struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func LoadCatalog() (Catalog, error) {
	var catalog Catalog
	decoder := json.NewDecoder(strings.NewReader(string(recipeSource)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil {
		return Catalog{}, fmt.Errorf("读取外部随包配方失败：%w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Catalog{}, fmt.Errorf("读取外部随包配方失败：%w", err)
	}
	if err := validateCatalog(catalog); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

func (c Catalog) Recipe(name string) (Recipe, error) {
	name = strings.TrimSpace(name)
	for _, recipe := range c.Recipes {
		if recipe.Name == name {
			return recipe, nil
		}
	}
	return Recipe{}, fmt.Errorf("外部随包配方 %q 不存在", name)
}

func (c Catalog) RecipesForArtifact(artifact types.ReleaseArtifactIdentity) []Recipe {
	target := artifact.Kind + ":" + artifact.ID
	result := make([]Recipe, 0, 1)
	for _, recipe := range c.Recipes {
		for _, candidate := range recipe.Artifacts {
			if candidate == target {
				result = append(result, recipe)
				break
			}
		}
	}
	sort.Slice(result, func(i int, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func validateCatalog(catalog Catalog) error {
	if catalog.SchemaVersion != manifestSchemaVersion {
		return fmt.Errorf("外部随包配方 schemaVersion 必须为 %d", manifestSchemaVersion)
	}
	if catalog.Platform != types.ReleasePlatformWindowsX64 {
		return fmt.Errorf("外部随包配方平台必须为 %s", types.ReleasePlatformWindowsX64)
	}
	seen := map[string]struct{}{}
	for index, recipe := range catalog.Recipes {
		if recipe.Name == "" || strings.ContainsAny(recipe.Name, `/\\`) {
			return fmt.Errorf("外部随包配方名称无效")
		}
		if _, exists := seen[recipe.Name]; exists {
			return fmt.Errorf("外部随包配方 %s 重复", recipe.Name)
		}
		seen[recipe.Name] = struct{}{}
		if index > 0 && catalog.Recipes[index-1].Name >= recipe.Name {
			return fmt.Errorf("外部随包配方必须按名称排序")
		}
		switch recipe.Kind {
		case "existing", "git-bash", "python-science", "powershell", "nushell":
		default:
			return fmt.Errorf("外部随包配方 %s 的类别无效", recipe.Name)
		}
		if strings.TrimSpace(recipe.Version) == "" || !validHTTPSURL(recipe.Source) {
			return fmt.Errorf("外部随包配方 %s 的版本或官方来源无效", recipe.Name)
		}
		if len(recipe.Sources) == 0 || len(recipe.Artifacts) == 0 || len(recipe.RequiredFiles) == 0 {
			return fmt.Errorf("外部随包配方 %s 的来源、发布物或必需文件不完整", recipe.Name)
		}
		for _, source := range recipe.Sources {
			if !validHTTPSURL(source) {
				return fmt.Errorf("外部随包配方 %s 包含无效来源", recipe.Name)
			}
		}
		if recipe.Kind == "existing" {
			if !safeRelativePath(recipe.RepositoryPath) || len(recipe.Inputs) != 0 {
				return fmt.Errorf("仓库内外部内容配方 %s 的目录或输入无效", recipe.Name)
			}
		} else if recipe.RepositoryPath != "" || len(recipe.Inputs) == 0 {
			return fmt.Errorf("可准备外部内容配方 %s 的目录或输入无效", recipe.Name)
		}
		for inputIndex, input := range recipe.Inputs {
			if inputIndex > 0 && recipe.Inputs[inputIndex-1].Name >= input.Name {
				return fmt.Errorf("外部随包配方 %s 的输入必须按名称排序", recipe.Name)
			}
			if filepath.Base(input.Name) != input.Name || !validHTTPSURL(input.URL) || input.Size <= 0 || !validSHA256(input.SHA256) {
				return fmt.Errorf("外部随包配方 %s 的输入 %s 无效", recipe.Name, input.Name)
			}
		}
		for _, required := range recipe.RequiredFiles {
			if !safeRelativePath(required.Path) || required.Size <= 0 {
				return fmt.Errorf("外部随包配方 %s 的必需文件资料无效", recipe.Name)
			}
			// A generated tree may pin the remaining files through verified source inputs.
			if required.SHA256 != "" && !validSHA256(required.SHA256) {
				return fmt.Errorf("外部随包配方 %s 的必需文件指纹无效", recipe.Name)
			}
		}
	}
	return nil
}

func validHTTPSURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func safeRelativePath(value string) bool {
	value = filepath.Clean(filepath.FromSlash(strings.TrimSpace(value)))
	return value != "." && value != ".." && !filepath.IsAbs(value) && filepath.VolumeName(value) == "" && !strings.HasPrefix(value, ".."+string(filepath.Separator))
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("存在多余内容")
		}
		return err
	}
	return nil
}
