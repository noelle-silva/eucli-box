package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type options struct {
	tool               string
	dataDir            string
	migrateLayout      bool
	buildTime          time.Time
	assetRoots         assetRootFlags
	requiredAssetRoots requiredAssetRootFlags
}

type assetRootFlags map[string]string

type requiredAssetRootFlags map[string]struct{}

func (f *assetRootFlags) String() string {
	return fmt.Sprint(map[string]string(*f))
}

func (f *assetRootFlags) Set(value string) error {
	name, root, ok := strings.Cut(value, "=")
	name = strings.TrimSpace(name)
	root = strings.TrimSpace(root)
	if !ok || name == "" || root == "" {
		return fmt.Errorf("asset root must use name=path")
	}
	if *f == nil {
		*f = assetRootFlags{}
	}
	(*f)[name] = root
	return nil
}

func (f *requiredAssetRootFlags) String() string {
	keys := make([]string, 0, len(*f))
	for key := range *f {
		keys = append(keys, key)
	}
	return strings.Join(keys, ",")
}

func (f *requiredAssetRootFlags) Set(value string) error {
	name := strings.TrimSpace(value)
	if name == "" {
		return fmt.Errorf("required asset root name is required")
	}
	if *f == nil {
		*f = requiredAssetRootFlags{}
	}
	(*f)[name] = struct{}{}
	return nil
}

func run(ctx context.Context, args []string) error {
	var opts options
	flags := flag.NewFlagSet("build-tools", flag.ContinueOnError)
	flags.StringVar(&opts.tool, "tool", "all", "tool id to build, or all")
	flags.StringVar(&opts.dataDir, "data-dir", "data", "runtime data directory")
	flags.BoolVar(&opts.migrateLayout, "migrate-layout", false, "move existing tool bodies and tool data into the current layout")
	buildTimeValue := flags.String("build-time", "", "stable RFC3339 build time; defaults to the current time")
	flags.Var(&opts.assetRoots, "asset-root", "tool asset root in name=path form; repeatable")
	flags.Var(&opts.requiredAssetRoots, "require-asset-root", "asset root name that must be provided when declared by a matched tool; repeatable")
	if err := flags.Parse(args); err != nil {
		return err
	}
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	tools, err := discoverTools(repoRoot, opts.tool)
	if err != nil {
		return err
	}
	if len(tools) == 0 {
		return fmt.Errorf("no tools matched %q", opts.tool)
	}
	dataDirInput := opts.dataDir
	if !filepath.IsAbs(dataDirInput) {
		dataDirInput = filepath.Join(repoRoot, dataDirInput)
	}
	dataDir, err := filepath.Abs(dataDirInput)
	if err != nil {
		return fmt.Errorf("resolve data dir: %w", err)
	}
	if opts.migrateLayout {
		if opts.tool != "all" {
			return fmt.Errorf("-migrate-layout requires -tool all so the layout is activated as one verified unit")
		}
		return migrateToolLayout(ctx, dataDir, tools)
	}
	buildTime, err := resolveBuildTime(*buildTimeValue)
	if err != nil {
		return err
	}
	opts.buildTime = buildTime
	for _, source := range tools {
		if err := buildTool(ctx, repoRoot, dataDir, source, opts); err != nil {
			return err
		}
	}
	return nil
}

type toolSource struct {
	ID  string
	Dir string
}

func discoverTools(repoRoot string, selectedTool string) ([]toolSource, error) {
	entries, err := os.ReadDir(filepath.Join(repoRoot, "tools"))
	if err != nil {
		return nil, fmt.Errorf("scan tools directory: %w", err)
	}
	tools := []toolSource{}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "cmd" {
			continue
		}
		id := entry.Name()
		if selectedTool != "all" && selectedTool != id {
			continue
		}
		toolDir := filepath.Join(repoRoot, "tools", id)
		if _, err := os.Stat(filepath.Join(toolDir, "tool.json")); err != nil {
			continue
		}
		tools = append(tools, toolSource{ID: id, Dir: toolDir})
	}
	return tools, nil
}

func buildTool(ctx context.Context, repoRoot string, dataDir string, source toolSource, opts options) error {
	definition, err := readToolDefinition(source)
	if err != nil {
		return err
	}
	if definition.ID != source.ID {
		return fmt.Errorf("tool %q tool.json id must match directory name", source.ID)
	}
	toolpack, hasToolpack, err := readToolpack(source.Dir)
	if err != nil {
		return err
	}
	bodyRoot := filepath.Join(dataDir, "tool-bodies")
	if err := os.MkdirAll(bodyRoot, 0o755); err != nil {
		return fmt.Errorf("create tool body root: %w", err)
	}
	targetDir := filepath.Join(bodyRoot, source.ID)
	if !pathWithin(bodyRoot, targetDir) {
		return fmt.Errorf("tool body target escapes tool body root")
	}
	stagingDir, err := os.MkdirTemp(bodyRoot, ".staging-"+source.ID+"-")
	if err != nil {
		return fmt.Errorf("create tool body staging directory: %w", err)
	}
	defer os.RemoveAll(stagingDir)
	if err := copyDirIfExists(filepath.Join(source.Dir, "providers"), filepath.Join(stagingDir, "providers")); err != nil {
		return err
	}
	if hasToolpack {
		if err := copyDeclaredAssetRoots(stagingDir, toolpack, opts.assetRoots, opts.requiredAssetRoots); err != nil {
			return err
		}
		updatedDefinition, err := writeRuntimeConfig(source.Dir, stagingDir, definition, toolpack)
		if err != nil {
			return err
		}
		definition = updatedDefinition
	} else if err := copyIfExists(filepath.Join(source.Dir, "config.json"), filepath.Join(stagingDir, "config.json")); err != nil {
		return err
	}
	binaryRelPath := filepath.ToSlash(filepath.Join("binary", runtime.GOOS+"-"+runtime.GOARCH, executableName(source.ID)))
	binaryAbsPath := filepath.Join(stagingDir, filepath.FromSlash(binaryRelPath))
	if err := os.MkdirAll(filepath.Dir(binaryAbsPath), 0o755); err != nil {
		return fmt.Errorf("create binary directory: %w", err)
	}
	packagePath := "./" + filepath.ToSlash(filepath.Join("tools", source.ID, "cmd", source.ID))
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", binaryAbsPath, packagePath)
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build %s: %w", source.ID, err)
	}
	definition.BodyDirectory = "."
	definition.DataDirectory = ""
	definition.UserConfig = nil
	definition.PromptDescriptionOverride = ""
	definition.Compatibility = types.CompatibilityStatus{}
	definition.Status = ""
	definition.StatusMessage = ""
	definition.Binaries = []types.ToolBinary{{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Path: binaryRelPath}}
	if definition.CreatedAt.IsZero() {
		definition.CreatedAt = opts.buildTime
	}
	definition.UpdatedAt = opts.buildTime
	if err := writeJSON(filepath.Join(stagingDir, "definition.json"), definition); err != nil {
		return err
	}
	if err := replaceDirectory(targetDir, stagingDir); err != nil {
		return fmt.Errorf("activate tool body: %w", err)
	}
	fmt.Printf("built tool %s -> %s\n", source.ID, targetDir)
	return nil
}

type toolpackSpec struct {
	AssetRoots    []assetRootSpec   `json:"assetRoots"`
	RuntimeConfig runtimeConfigSpec `json:"runtimeConfig"`
	DataPaths     []string          `json:"dataPaths,omitempty"`
}

type assetRootSpec struct {
	Name              string   `json:"name"`
	Target            string   `json:"target"`
	RequiredFile      string   `json:"requiredFile"`
	RequiredFiles     []string `json:"requiredFiles,omitempty"`
	RequiredInPackage bool     `json:"requiredInPackage,omitempty"`
	Required          bool     `json:"required,omitempty"`
}

type runtimeConfigSpec struct {
	Source           string `json:"source"`
	ProviderArgument string `json:"providerArgument"`
}

func readToolpack(sourceDir string) (toolpackSpec, bool, error) {
	path := filepath.Join(sourceDir, "toolpack.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return toolpackSpec{}, false, nil
		}
		return toolpackSpec{}, false, fmt.Errorf("read toolpack.json: %w", err)
	}
	var spec toolpackSpec
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		return toolpackSpec{}, false, fmt.Errorf("decode toolpack.json: %w", err)
	}
	return spec, true, nil
}

func copyDeclaredAssetRoots(targetDir string, toolpack toolpackSpec, roots assetRootFlags, requiredRoots requiredAssetRootFlags) error {
	for _, asset := range toolpack.AssetRoots {
		requiredFiles, err := validateAssetRootSpec(asset)
		if err != nil {
			return err
		}
		rootInput := strings.TrimSpace(roots[asset.Name])
		if rootInput == "" {
			if asset.Required || assetRootRequired(requiredRoots, asset.Name) {
				return fmt.Errorf("required asset root %q was not provided", asset.Name)
			}
			continue
		}
		root, err := filepath.Abs(rootInput)
		if err != nil {
			return fmt.Errorf("resolve asset root %q: %w", asset.Name, err)
		}
		info, err := os.Stat(root)
		if err != nil {
			return fmt.Errorf("stat asset root %q: %w", asset.Name, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("asset root %q is not a directory", asset.Name)
		}
		if err := validateRequiredAssetFiles(root, asset.Name, requiredFiles, "asset root"); err != nil {
			return err
		}
		target := filepath.Join(targetDir, filepath.FromSlash(asset.Target))
		if !pathWithin(targetDir, target) {
			return fmt.Errorf("asset root %q target escapes tool package", asset.Name)
		}
		if err := copyDir(root, target); err != nil {
			return err
		}
	}
	return validatePackagedAssetRoots(targetDir, toolpack)
}

func assetRootRequired(requiredRoots requiredAssetRootFlags, name string) bool {
	if len(requiredRoots) == 0 {
		return false
	}
	_, ok := requiredRoots[name]
	return ok
}

func validateAssetRootSpec(asset assetRootSpec) ([]string, error) {
	requiredFiles, err := requiredAssetFiles(asset)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(asset.Name) == "" || strings.TrimSpace(asset.Target) == "" || len(requiredFiles) == 0 {
		return nil, fmt.Errorf("toolpack assetRoots entries require name, target, and requiredFile or requiredFiles")
	}
	if !isRelativePackagePath(asset.Target) {
		return nil, fmt.Errorf("toolpack asset root %q paths must be relative", asset.Name)
	}
	for _, requiredFile := range requiredFiles {
		if !isRelativePackagePath(requiredFile) {
			return nil, fmt.Errorf("toolpack asset root %q paths must be relative", asset.Name)
		}
	}
	return requiredFiles, nil
}

func requiredAssetFiles(asset assetRootSpec) ([]string, error) {
	requiredFiles := []string{}
	if requiredFile := strings.TrimSpace(asset.RequiredFile); requiredFile != "" {
		requiredFiles = append(requiredFiles, requiredFile)
	}
	for _, requiredFile := range asset.RequiredFiles {
		requiredFile = strings.TrimSpace(requiredFile)
		if requiredFile == "" {
			return nil, fmt.Errorf("toolpack asset root %q requiredFiles entries must not be empty", asset.Name)
		}
		requiredFiles = append(requiredFiles, requiredFile)
	}
	return requiredFiles, nil
}

func isRelativePackagePath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	path = filepath.Clean(filepath.FromSlash(path))
	if filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
		return false
	}
	return path != ".." && !strings.HasPrefix(path, ".."+string(filepath.Separator))
}

func validatePackagedAssetRoots(targetDir string, toolpack toolpackSpec) error {
	for _, asset := range toolpack.AssetRoots {
		requiredFiles, err := validateAssetRootSpec(asset)
		if err != nil {
			return err
		}
		target := filepath.Join(targetDir, filepath.FromSlash(asset.Target))
		if !pathWithin(targetDir, target) {
			return fmt.Errorf("asset root %q target escapes tool package", asset.Name)
		}
		info, err := os.Stat(target)
		if err != nil {
			if os.IsNotExist(err) {
				if asset.RequiredInPackage {
					return fmt.Errorf("required packaged asset root %q was not produced", asset.Name)
				}
				continue
			}
			return fmt.Errorf("stat packaged asset root %q: %w", asset.Name, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("packaged asset root %q is not a directory", asset.Name)
		}
		if err := validateRequiredAssetFiles(target, asset.Name, requiredFiles, "packaged asset root"); err != nil {
			return err
		}
	}
	return nil
}

func validateRequiredAssetFiles(base string, assetName string, requiredFiles []string, label string) error {
	for _, requiredFile := range requiredFiles {
		requiredPath := filepath.Join(base, filepath.FromSlash(requiredFile))
		if !pathWithin(base, requiredPath) {
			return fmt.Errorf("%s %q required file %s escapes asset root", label, assetName, requiredFile)
		}
		requiredInfo, err := os.Stat(requiredPath)
		if err != nil {
			return fmt.Errorf("%s %q must contain %s", label, assetName, requiredFile)
		}
		if requiredInfo.IsDir() {
			return fmt.Errorf("%s %q required file %s is a directory", label, assetName, requiredFile)
		}
	}
	return nil
}

func writeRuntimeConfig(sourceDir string, targetDir string, definition types.ToolDefinition, toolpack toolpackSpec) (types.ToolDefinition, error) {
	if strings.TrimSpace(toolpack.RuntimeConfig.Source) == "" {
		return definition, nil
	}
	source := filepath.Join(sourceDir, toolpack.RuntimeConfig.Source)
	config, err := readRuntimeConfig(source)
	if err != nil {
		return types.ToolDefinition{}, err
	}
	if strings.TrimSpace(toolpack.RuntimeConfig.ProviderArgument) == "" {
		if err := writeJSON(filepath.Join(targetDir, toolpack.RuntimeConfig.Source), config); err != nil {
			return types.ToolDefinition{}, err
		}
		return definition, nil
	}
	providers, err := runtimeProviders(config)
	if err != nil {
		return types.ToolDefinition{}, err
	}
	if defaultProvider(config) == "" {
		return types.ToolDefinition{}, fmt.Errorf("runtime config defaultProvider is required")
	}
	enabledProviders := []string{}
	defaultAvailable := false
	for index := range providers {
		provider := &providers[index]
		available := providerExecutableExists(targetDir, *provider)
		enabled := providerEnabled(*provider) && available
		(*provider)["enabled"] = enabled
		if enabled {
			enabledProviders = append(enabledProviders, providerID(*provider))
		}
		if providerID(*provider) == defaultProvider(config) && enabled {
			defaultAvailable = true
		}
	}
	if !defaultAvailable {
		return types.ToolDefinition{}, fmt.Errorf("default provider %q is enabled but its bundled executable is missing", defaultProvider(config))
	}
	if len(enabledProviders) == 0 {
		return types.ToolDefinition{}, fmt.Errorf("tool must package at least one enabled provider")
	}
	config["providers"] = providers
	definition.InputSchema = withEnabledProviderEnum(definition.InputSchema, toolpack.RuntimeConfig.ProviderArgument, enabledProviders)
	if definition.UserConfigSchema != nil {
		definition.UserConfigSchema = withEnabledProviderEnum(definition.UserConfigSchema, toolpack.RuntimeConfig.ProviderArgument, enabledProviders)
	}
	if err := writeJSON(filepath.Join(targetDir, toolpack.RuntimeConfig.Source), config); err != nil {
		return types.ToolDefinition{}, err
	}
	return definition, nil
}

func readRuntimeConfig(path string) (map[string]any, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read runtime config: %w", err)
	}
	var config map[string]any
	if err := json.Unmarshal(payload, &config); err != nil {
		return nil, fmt.Errorf("decode runtime config: %w", err)
	}
	return config, nil
}

func runtimeProviders(config map[string]any) ([]map[string]any, error) {
	rawProviders, ok := config["providers"].([]any)
	if !ok {
		return nil, fmt.Errorf("runtime config providers are required")
	}
	providers := make([]map[string]any, 0, len(rawProviders))
	for _, rawProvider := range rawProviders {
		provider, ok := rawProvider.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("runtime config provider entry must be an object")
		}
		providers = append(providers, provider)
	}
	if len(providers) == 0 {
		return nil, fmt.Errorf("runtime config providers are required")
	}
	return providers, nil
}

func defaultProvider(config map[string]any) string {
	value, _ := config["defaultProvider"].(string)
	return strings.TrimSpace(value)
}

func providerID(provider map[string]any) string {
	value, _ := provider["id"].(string)
	return strings.TrimSpace(value)
}

func providerEnabled(provider map[string]any) bool {
	value, _ := provider["enabled"].(bool)
	return value
}

func providerMode(provider map[string]any) string {
	value, _ := provider["mode"].(string)
	return strings.TrimSpace(value)
}

func providerExecutables(provider map[string]any) ([]types.ToolBinary, error) {
	payload, err := json.Marshal(provider["executables"])
	if err != nil {
		return nil, err
	}
	var executables []types.ToolBinary
	if err := json.Unmarshal(payload, &executables); err != nil {
		return nil, err
	}
	return executables, nil
}

func providerExecutableExists(targetDir string, provider map[string]any) bool {
	executables, err := providerExecutables(provider)
	if err != nil {
		return false
	}
	if providerID(provider) == "" || providerMode(provider) != "bundled" || len(executables) == 0 {
		return false
	}
	for _, executable := range executables {
		if executable.GOOS != runtime.GOOS || executable.GOARCH != runtime.GOARCH || strings.TrimSpace(executable.Path) == "" {
			continue
		}
		if filepath.IsAbs(executable.Path) || filepath.VolumeName(executable.Path) != "" {
			return false
		}
		path := filepath.Join(targetDir, filepath.FromSlash(executable.Path))
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func withEnabledProviderEnum(schema map[string]any, providerArgument string, enabledProviders []string) map[string]any {
	if schema == nil {
		schema = map[string]any{}
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		properties = map[string]any{}
		schema["properties"] = properties
	}
	provider, ok := properties[providerArgument].(map[string]any)
	if !ok {
		provider = map[string]any{}
		properties[providerArgument] = provider
	}
	values := make([]string, 0, len(enabledProviders))
	values = append(values, enabledProviders...)
	provider["enum"] = values
	return schema
}

func readToolDefinition(source toolSource) (types.ToolDefinition, error) {
	payload, err := os.ReadFile(filepath.Join(source.Dir, "tool.json"))
	if err != nil {
		return types.ToolDefinition{}, fmt.Errorf("read %s tool.json: %w", source.ID, err)
	}
	var definition types.ToolDefinition
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&definition); err != nil {
		return types.ToolDefinition{}, fmt.Errorf("decode %s tool.json: %w", source.ID, err)
	}
	if strings.TrimSpace(definition.ID) == "" || strings.TrimSpace(definition.Name) == "" || strings.TrimSpace(definition.Description) == "" {
		return types.ToolDefinition{}, fmt.Errorf("tool %s must declare id, name, and description", source.ID)
	}
	if !types.ValidExplicitToolInvocationMode(definition.DefaultInvocationMode) {
		return types.ToolDefinition{}, fmt.Errorf("tool %s defaultInvocationMode must be sync or async", source.ID)
	}
	definition.DefaultInvocationMode = types.CleanToolInvocationMode(definition.DefaultInvocationMode)
	if definition.Type != "local" {
		return types.ToolDefinition{}, fmt.Errorf("tool %s type must be local", source.ID)
	}
	if err := validateReleaseMetadata(definition.Version, definition.EucliBoxCompatibility); err != nil {
		return types.ToolDefinition{}, fmt.Errorf("tool %s release metadata: %w", source.ID, err)
	}
	return definition, nil
}

func validateReleaseMetadata(version string, compatibility types.EucliBoxCompatibility) error {
	if err := release.ValidateVersion(version); err != nil {
		return err
	}
	return release.ValidateEucliBoxCompatibility(compatibility)
}

func copyIfExists(source string, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", source, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", source)
	}
	return copyFile(source, target, info.Mode())
}

func copyDirIfExists(source string, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", source, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", source)
	}
	return copyDir(source, target)
}

func copyDir(source string, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return copyFile(path, targetPath, info.Mode())
	})
}

func copyFile(source string, target string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open %s: %w", source, err)
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create parent for %s: %w", target, err)
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("open %s: %w", target, err)
	}
	defer output.Close()
	if _, err := io.Copy(output, input); err != nil {
		return fmt.Errorf("copy %s to %s: %w", source, target, err)
	}
	return nil
}

func writeJSON(target string, value any) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create parent for %s: %w", target, err)
	}
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", target, err)
	}
	payload = append(payload, '\n')
	return os.WriteFile(target, payload, 0o644)
}

func replaceDirectory(target string, staging string) error {
	backup := target + ".previous"
	if err := os.RemoveAll(backup); err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(staging, target); err != nil {
		if rollbackErr := os.Rename(backup, target); rollbackErr != nil {
			return fmt.Errorf("activate staged directory: %w; rollback failed: %v", err, rollbackErr)
		}
		return err
	}
	return os.RemoveAll(backup)
}

func executableName(toolID string) string {
	if runtime.GOOS == "windows" {
		return toolID + ".exe"
	}
	return toolID
}

func findRepoRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("go.mod was not found")
		}
		current = parent
	}
}

func pathWithin(base string, child string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(child))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
