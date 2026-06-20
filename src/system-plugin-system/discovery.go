package systemplugin

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"eucli-box/pkg/types"
)

type pluginRecord struct {
	manifest      types.SystemPluginManifest
	defaultConfig map[string]any
	userConfig    types.SystemPluginUserConfig
	directory     string
	executable    string
	status        string
	statusMessage string
}

func (s *system) ListPlugins(ctx context.Context) ([]types.SystemPluginSummary, error) {
	records, err := s.discover(ctx)
	if err != nil {
		return nil, err
	}
	summaries := make([]types.SystemPluginSummary, 0, len(records))
	for _, record := range records {
		summaries = append(summaries, types.SystemPluginSummary{ID: record.manifest.ID, Name: record.manifest.Name, Description: record.manifest.Description, Version: record.manifest.Version, LifecycleType: record.manifest.LifecycleType, Status: record.status, StatusMessage: record.statusMessage})
	}
	return summaries, nil
}

func (s *system) LoadPlugin(ctx context.Context, pluginID string) (types.SystemPluginView, error) {
	record, err := s.findRecord(ctx, pluginID)
	if err != nil {
		return types.SystemPluginView{}, err
	}
	return record.view(), nil
}

func (s *system) discover(ctx context.Context) ([]pluginRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, pluginReadFailed("read cancelled", err)
	}
	entries, err := os.ReadDir(s.sourceDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, pluginReadFailed("failed to read system plugin source directory", err)
	}
	records := []pluginRecord{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		record, ok, err := s.loadRecord(ctx, filepath.Join(s.sourceDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if ok {
			records = append(records, record)
		}
	}
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].manifest.Name < records[j].manifest.Name
	})
	return records, nil
}

func (s *system) loadRecord(ctx context.Context, directory string) (pluginRecord, bool, error) {
	manifestFile := filepath.Join(directory, "manifest.json")
	if !fileExists(manifestFile) {
		return pluginRecord{}, false, nil
	}
	manifest, err := readJSONFile[types.SystemPluginManifest](ctx, manifestFile)
	if err != nil {
		return pluginRecord{}, false, pluginReadFailed("failed to read system plugin manifest", err)
	}
	manifest = normalizeManifest(manifest)
	if err := validateManifest(manifest); err != nil {
		return pluginRecord{}, false, err
	}
	defaultConfig, err := readMapFile(ctx, filepath.Join(directory, "config.json"))
	if err != nil {
		return pluginRecord{}, false, err
	}
	userConfig, err := s.loadUserConfig(ctx, manifest.ID)
	if err != nil {
		return pluginRecord{}, false, err
	}
	record := pluginRecord{manifest: manifest, defaultConfig: defaultConfig, userConfig: userConfig, directory: directory, status: types.SystemPluginStatusActive}
	if executable, err := selectExecutable(directory, manifest.Binaries); err != nil {
		record.status = types.SystemPluginStatusUnavailable
		record.statusMessage = err.Error()
	} else {
		record.executable = executable
	}
	if failure := s.getFailure(manifest.ID); failure != "" {
		record.status = types.SystemPluginStatusUnavailable
		record.statusMessage = failure
	}
	return record, true, nil
}

func (s *system) findRecord(ctx context.Context, pluginID string) (pluginRecord, error) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return pluginRecord{}, pluginInvalid("system plugin id is required", nil)
	}
	records, err := s.discover(ctx)
	if err != nil {
		return pluginRecord{}, err
	}
	for _, record := range records {
		if record.manifest.ID == pluginID {
			return record, nil
		}
	}
	return pluginRecord{}, pluginInvalid("system plugin was not found", nil)
}

func normalizeManifest(manifest types.SystemPluginManifest) types.SystemPluginManifest {
	manifest.ID = strings.TrimSpace(manifest.ID)
	manifest.Name = strings.TrimSpace(manifest.Name)
	manifest.Description = strings.TrimSpace(manifest.Description)
	manifest.Version = strings.TrimSpace(manifest.Version)
	manifest.LifecycleType = strings.TrimSpace(manifest.LifecycleType)
	for index := range manifest.Binaries {
		manifest.Binaries[index].GOOS = strings.TrimSpace(manifest.Binaries[index].GOOS)
		manifest.Binaries[index].GOARCH = strings.TrimSpace(manifest.Binaries[index].GOARCH)
		manifest.Binaries[index].Path = strings.TrimSpace(manifest.Binaries[index].Path)
	}
	for index := range manifest.PlaceholderInterfaces {
		manifest.PlaceholderInterfaces[index].ID = strings.TrimSpace(manifest.PlaceholderInterfaces[index].ID)
		manifest.PlaceholderInterfaces[index].DefaultName = strings.TrimSpace(manifest.PlaceholderInterfaces[index].DefaultName)
		manifest.PlaceholderInterfaces[index].Description = strings.TrimSpace(manifest.PlaceholderInterfaces[index].Description)
	}
	return manifest
}

func validateManifest(manifest types.SystemPluginManifest) error {
	if manifest.ID == "" {
		return pluginInvalid("system plugin manifest id is required", nil)
	}
	if manifest.Name == "" {
		return pluginInvalid("system plugin manifest name is required", nil)
	}
	if manifest.Description == "" {
		return pluginInvalid("system plugin manifest description is required", nil)
	}
	if manifest.LifecycleType != types.SystemPluginLifecyclePersistent && manifest.LifecycleType != types.SystemPluginLifecycleOnDemand {
		return pluginInvalid("system plugin lifecycleType must be persistent or on-demand", nil)
	}
	if len(manifest.Binaries) == 0 {
		return pluginInvalid("system plugin manifest must declare at least one binary", nil)
	}
	seenInterfaces := map[string]struct{}{}
	for _, item := range manifest.PlaceholderInterfaces {
		if item.ID == "" || item.DefaultName == "" {
			return pluginInvalid("system plugin placeholder interface id and defaultName are required", nil)
		}
		if _, ok := seenInterfaces[item.ID]; ok {
			return pluginInvalid("system plugin placeholder interface id must be unique", nil)
		}
		seenInterfaces[item.ID] = struct{}{}
	}
	return nil
}

func selectExecutable(directory string, binaries []types.SystemPluginBinary) (string, error) {
	for _, binary := range binaries {
		if binary.GOOS != runtime.GOOS || binary.GOARCH != runtime.GOARCH {
			continue
		}
		if binary.Path == "" {
			return "", pluginInvalid("system plugin binary path is required", nil)
		}
		executable := filepath.Join(directory, filepath.Clean(binary.Path))
		absDirectory, err := filepath.Abs(directory)
		if err != nil {
			return "", pluginInvalid("failed to resolve system plugin directory", err)
		}
		absExecutable, err := filepath.Abs(executable)
		if err != nil {
			return "", pluginInvalid("failed to resolve system plugin executable", err)
		}
		if !pathWithin(absDirectory, absExecutable) {
			return "", pluginInvalid("system plugin executable must stay inside plugin directory", nil)
		}
		info, err := os.Stat(absExecutable)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			return "", pluginInvalid("system plugin executable is a directory", nil)
		}
		return filepath.Clean(absExecutable), nil
	}
	return "", pluginInvalid("system plugin has no binary for current platform", nil)
}

func (r pluginRecord) view() types.SystemPluginView {
	return types.SystemPluginView{ID: r.manifest.ID, Name: r.manifest.Name, Description: r.manifest.Description, Version: r.manifest.Version, LifecycleType: r.manifest.LifecycleType, Status: r.status, StatusMessage: r.statusMessage, DefaultConfig: copyMap(r.defaultConfig), UserConfig: copyMap(r.userConfig.UserConfig), ConfigSchema: copyMap(r.manifest.ConfigSchema), PlaceholderInterfaces: r.interfaceViews()}
}

func (r pluginRecord) interfaceViews() []types.SystemPluginPlaceholderInterfaceView {
	out := make([]types.SystemPluginPlaceholderInterfaceView, 0, len(r.manifest.PlaceholderInterfaces))
	for _, item := range r.manifest.PlaceholderInterfaces {
		out = append(out, types.SystemPluginPlaceholderInterfaceView{ID: item.ID, DefaultName: item.DefaultName, EffectiveName: r.effectiveName(item), Description: item.Description})
	}
	return out
}

func (r pluginRecord) effectiveName(item types.SystemPluginPlaceholderInterface) string {
	if override := strings.TrimSpace(r.userConfig.PlaceholderNameOverrides[item.ID]); override != "" {
		return override
	}
	return item.DefaultName
}

func readJSONFile[T any](ctx context.Context, path string) (T, error) {
	var value T
	if err := ctx.Err(); err != nil {
		return value, err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(payload, &value); err != nil {
		return value, err
	}
	return value, nil
}

func readMapFile(ctx context.Context, path string) (map[string]any, error) {
	if !fileExists(path) {
		return nil, pluginReadFailed("system plugin config.json is required", nil)
	}
	value, err := readJSONFile[map[string]any](ctx, path)
	if err != nil {
		return nil, pluginReadFailed("failed to read system plugin config", err)
	}
	return value, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func pathWithin(base string, child string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(child))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func copyMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
