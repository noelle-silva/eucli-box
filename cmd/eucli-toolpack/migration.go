package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"eucli-box/pkg/types"
)

type legacyToolDefinition struct {
	types.ToolDefinition
	Directory string `json:"directory"`
}

func migrateToolLayout(ctx context.Context, dataDir string, tools []toolSource) error {
	legacyRoot := filepath.Join(dataDir, "tools")
	if err := validateLegacyToolSet(legacyRoot, tools); err != nil {
		return err
	}
	stagingRoot, err := os.MkdirTemp(dataDir, ".tool-layout-staging-")
	if err != nil {
		return fmt.Errorf("create tool layout staging directory: %w", err)
	}
	defer os.RemoveAll(stagingRoot)

	stagedBodiesRoot := filepath.Join(stagingRoot, "tool-bodies")
	stagedDataRoot := filepath.Join(stagingRoot, "tool-data")
	for _, source := range tools {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("tool layout migration cancelled: %w", err)
		}
		if err := stageLegacyTool(ctx, legacyRoot, stagedBodiesRoot, stagedDataRoot, source); err != nil {
			return err
		}
	}
	if err := activateToolLayout(dataDir, stagedBodiesRoot, stagedDataRoot); err != nil {
		return err
	}
	for _, source := range tools {
		fmt.Printf("migrated tool %s body and data\n", source.ID)
	}
	fmt.Printf("verified tool layout activated; legacy source retained at %s for manual inspection\n", legacyRoot)
	return nil
}

func validateLegacyToolSet(legacyRoot string, tools []toolSource) error {
	entries, err := os.ReadDir(legacyRoot)
	if err != nil {
		return fmt.Errorf("read legacy tool directory: %w", err)
	}
	expected := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		expected[tool.ID] = struct{}{}
	}
	actual := map[string]struct{}{}
	for _, entry := range entries {
		if entry.IsDir() {
			actual[entry.Name()] = struct{}{}
		}
	}
	for id := range expected {
		if _, ok := actual[id]; !ok {
			return fmt.Errorf("legacy tool %q is missing", id)
		}
	}
	for id := range actual {
		if _, ok := expected[id]; !ok {
			return fmt.Errorf("legacy tool %q has no matching source declaration", id)
		}
	}
	return nil
}

func stageLegacyTool(ctx context.Context, legacyRoot string, stagedBodiesRoot string, stagedDataRoot string, source toolSource) error {
	sourceDefinition, err := readToolDefinition(source)
	if err != nil {
		return err
	}
	legacyDir := filepath.Join(legacyRoot, source.ID)
	legacy, err := readLegacyToolDefinition(filepath.Join(legacyDir, "data.json"))
	if err != nil {
		return fmt.Errorf("read legacy tool %s definition: %w", source.ID, err)
	}
	if legacy.ID != source.ID {
		return fmt.Errorf("legacy tool %q definition id is %q", source.ID, legacy.ID)
	}
	if strings.TrimSpace(legacy.Directory) != "." {
		return fmt.Errorf("legacy tool %q directory must be . before migration", source.ID)
	}
	toolpack, hasToolpack, err := readToolpack(source.Dir)
	if err != nil {
		return err
	}
	dataPaths, err := validateDataPaths(toolpack.DataPaths)
	if err != nil {
		return fmt.Errorf("tool %s data paths: %w", source.ID, err)
	}
	bodyPaths, err := declaredBodyPaths(toolpack, hasToolpack)
	if err != nil {
		return fmt.Errorf("tool %s body paths: %w", source.ID, err)
	}
	if err := validateSeparatedPaths(bodyPaths, dataPaths); err != nil {
		return fmt.Errorf("tool %s layout: %w", source.ID, err)
	}
	if err := validateLegacyPaths(legacyDir, bodyPaths, dataPaths); err != nil {
		return fmt.Errorf("tool %s layout: %w", source.ID, err)
	}

	bodyDir := filepath.Join(stagedBodiesRoot, source.ID)
	dataDir := filepath.Join(stagedDataRoot, source.ID)
	if err := os.MkdirAll(bodyDir, 0o755); err != nil {
		return fmt.Errorf("create staged tool body: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create staged tool data: %w", err)
	}
	if err := copyLegacyBody(ctx, legacyDir, bodyDir, bodyPaths); err != nil {
		return fmt.Errorf("stage tool %s body: %w", source.ID, err)
	}
	for _, dataPath := range dataPaths {
		legacyPath := filepath.Join(legacyDir, dataPath)
		if _, err := os.Stat(legacyPath); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return fmt.Errorf("inspect tool %s data path %s: %w", source.ID, dataPath, err)
		}
		stagedPath := filepath.Join(dataDir, dataPath)
		if err := copyPath(legacyPath, stagedPath); err != nil {
			return fmt.Errorf("copy tool %s data path %s: %w", source.ID, dataPath, err)
		}
		if err := verifyCopiedPath(legacyPath, stagedPath); err != nil {
			return fmt.Errorf("verify tool %s data path %s: %w", source.ID, dataPath, err)
		}
	}

	settings := types.ToolUserSettings{
		UserConfig:                copyMap(legacy.UserConfig),
		PromptDescriptionOverride: strings.TrimSpace(legacy.PromptDescriptionOverride),
		UpdatedAt:                 legacy.UpdatedAt,
	}
	if err := writeJSON(filepath.Join(dataDir, "settings.json"), settings); err != nil {
		return err
	}
	definition := legacy.ToolDefinition
	definition.Version = sourceDefinition.Version
	definition.EucliBoxCompatibility = sourceDefinition.EucliBoxCompatibility
	definition.BodyDirectory = "."
	definition.DataDirectory = ""
	definition.UserConfig = nil
	definition.PromptDescriptionOverride = ""
	definition.Compatibility = types.CompatibilityStatus{}
	definition.Status = ""
	definition.StatusMessage = ""
	if err := writeJSON(filepath.Join(bodyDir, "definition.json"), definition); err != nil {
		return err
	}
	return verifyStagedTool(bodyDir, dataDir, source.ID, settings)
}

func readLegacyToolDefinition(path string) (legacyToolDefinition, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return legacyToolDefinition{}, err
	}
	var definition legacyToolDefinition
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&definition); err != nil {
		return legacyToolDefinition{}, err
	}
	return definition, nil
}

func validateDataPaths(paths []string) ([]string, error) {
	cleaned := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for _, value := range paths {
		path := filepath.Clean(filepath.FromSlash(strings.TrimSpace(value)))
		if path == "." || !isRelativePackagePath(path) || strings.EqualFold(path, "data.json") {
			return nil, fmt.Errorf("dataPaths entries must be non-empty relative paths outside data.json")
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		cleaned = append(cleaned, path)
	}
	sort.Strings(cleaned)
	for index, path := range cleaned {
		for _, parent := range cleaned[:index] {
			if pathWithin(parent, path) {
				return nil, fmt.Errorf("dataPaths entries must not overlap: %s and %s", parent, path)
			}
		}
	}
	return cleaned, nil
}

func declaredBodyPaths(toolpack toolpackSpec, hasToolpack bool) ([]string, error) {
	paths := []string{"binary", "providers"}
	if hasToolpack {
		if source := strings.TrimSpace(toolpack.RuntimeConfig.Source); source != "" {
			if !isRelativePackagePath(source) {
				return nil, fmt.Errorf("runtime config source must be relative")
			}
			paths = append(paths, filepath.Clean(filepath.FromSlash(source)))
		}
		for _, asset := range toolpack.AssetRoots {
			if _, err := validateAssetRootSpec(asset); err != nil {
				return nil, err
			}
			paths = append(paths, filepath.Clean(filepath.FromSlash(asset.Target)))
		}
	} else {
		paths = append(paths, "config.json")
	}
	return collapsePaths(paths), nil
}

func collapsePaths(paths []string) []string {
	sort.Strings(paths)
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		covered := false
		for _, root := range out {
			if path == root || pathWithin(root, path) {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, path)
		}
	}
	return out
}

func validateSeparatedPaths(bodyPaths []string, dataPaths []string) error {
	for _, bodyPath := range bodyPaths {
		for _, dataPath := range dataPaths {
			if bodyPath == dataPath || pathWithin(bodyPath, dataPath) || pathWithin(dataPath, bodyPath) {
				return fmt.Errorf("body path %s overlaps data path %s", bodyPath, dataPath)
			}
		}
	}
	return nil
}

func validateLegacyPaths(legacyDir string, bodyPaths []string, dataPaths []string) error {
	return filepath.WalkDir(legacyDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(legacyDir, path)
		if err != nil {
			return err
		}
		if relative == "." || strings.EqualFold(relative, "data.json") {
			return nil
		}
		if pathCoveredBy(relative, bodyPaths) || pathCoveredBy(relative, dataPaths) {
			return nil
		}
		if entry.IsDir() && (pathContainsDeclaredRoot(relative, bodyPaths) || pathContainsDeclaredRoot(relative, dataPaths)) {
			return nil
		}
		return fmt.Errorf("unrecognized legacy path %s", relative)
	})
}

func pathContainsDeclaredRoot(path string, roots []string) bool {
	for _, root := range roots {
		if pathWithin(path, root) {
			return true
		}
	}
	return false
}

func activateToolLayout(dataDir string, stagedBodiesRoot string, stagedDataRoot string) error {
	targets := []string{filepath.Join(dataDir, "tool-bodies"), filepath.Join(dataDir, "tool-data")}
	for _, target := range targets {
		if err := requireEmptyMigrationTarget(target); err != nil {
			return err
		}
	}
	for _, target := range targets {
		if err := removeEmptyMigrationTarget(target); err != nil {
			return err
		}
	}
	if err := os.Rename(stagedBodiesRoot, targets[0]); err != nil {
		return fmt.Errorf("activate tool bodies: %w", err)
	}
	if err := os.Rename(stagedDataRoot, targets[1]); err != nil {
		if rollbackErr := os.RemoveAll(targets[0]); rollbackErr != nil {
			return fmt.Errorf("activate tool data: %w; rollback tool bodies failed: %v", err, rollbackErr)
		}
		return fmt.Errorf("activate tool data: %w", err)
	}
	return nil
}

func requireEmptyMigrationTarget(path string) error {
	entries, err := os.ReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect migration target %s: %w", path, err)
	}
	if len(entries) != 0 {
		return fmt.Errorf("migration target %s is not empty", path)
	}
	return nil
}

func removeEmptyMigrationTarget(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("remove empty migration target %s: %w", path, err)
	}
	return nil
}

func copyMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
