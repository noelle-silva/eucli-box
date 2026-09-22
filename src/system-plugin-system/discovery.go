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

	"eucli-box/pkg/release"
	"eucli-box/pkg/systemplugin"
	"eucli-box/pkg/types"
)

type pluginRecord struct {
	manifest      types.SystemPluginManifest
	sourceID      string
	defaultConfig map[string]any
	userConfig    types.SystemPluginUserConfig
	directory     string
	dataDirectory string
	executable    string
	compatibility types.CompatibilityStatus
	status        string
	statusMessage string
	enabled       bool
}

func (s *system) ListPlugins(ctx context.Context) ([]types.SystemPluginSummary, error) {
	index, err := s.discover(ctx)
	if err != nil {
		return nil, err
	}
	summaries := make([]types.SystemPluginSummary, 0, len(index.records))
	for _, record := range index.records {
		statusMessage := record.statusMessage
		if failure := s.getFailure(record.manifest.ID); failure != "" && record.status == types.SystemPluginStatusActive {
			statusMessage = failure
		}
		summaries = append(summaries, types.SystemPluginSummary{
			ID:                    record.manifest.ID,
			SourceID:              record.sourceID,
			Name:                  record.displayName(),
			Description:           record.manifest.Description,
			Version:               record.manifest.Version,
			EucliBoxCompatibility: record.manifest.EucliBoxCompatibility,
			Compatibility:         record.compatibility,
			Hosting:               record.manifest.Hosting,
			Status:                record.status,
			StatusMessage:         statusMessage,
			Installed:             true,
			CurrentVersion:        record.manifest.Version,
			InstallStatus:         s.installStatusFor(record),
			Active:                s.activityFor(record.manifest.ID).state().Active,
			Enabled:               record.enabled,
		})
	}
	return summaries, nil
}

func (s *system) LoadPlugin(ctx context.Context, pluginID string) (types.SystemPluginView, error) {
	index, err := s.discover(ctx)
	if err != nil {
		return types.SystemPluginView{}, err
	}
	record, ok := index.find(pluginID)
	if !ok {
		return types.SystemPluginView{}, pluginInvalid("system plugin was not found", nil)
	}
	view := record.view()
	if failure := s.getFailure(record.manifest.ID); failure != "" && record.status == types.SystemPluginStatusActive {
		view.StatusMessage = failure
	}
	view.Active = s.activityFor(record.locatorID()).state().Active
	view.InstallStatus = s.installStatusFor(record)
	return view, nil
}

func (s *system) discover(ctx context.Context) (*pluginIndex, error) {
	if err := ctx.Err(); err != nil {
		return nil, pluginReadFailed("read cancelled", err)
	}
	var (
		records []pluginRecord
		err     error
	)
	if s.managedPrograms() {
		records, err = s.discoverManaged(ctx)
	} else {
		records, err = s.discoverDev(ctx)
	}
	if err != nil {
		return nil, err
	}
	return buildPluginIndex(records), nil
}

func (s *system) findRecord(ctx context.Context, pluginID string) (pluginRecord, error) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return pluginRecord{}, pluginInvalid("system plugin id is required", nil)
	}
	index, err := s.discover(ctx)
	if err != nil {
		return pluginRecord{}, err
	}
	record, ok := index.find(pluginID)
	if !ok {
		return pluginRecord{}, pluginInvalid("system plugin was not found", nil)
	}
	return record, nil
}

// discoverDev 是普通开发启动的插件发现：扫描 SourceDir 一级子目录。
func (s *system) discoverDev(ctx context.Context) ([]pluginRecord, error) {
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
		if err := ctx.Err(); err != nil {
			return nil, pluginReadFailed("read cancelled", err)
		}
		record, ok := s.loadRecord(ctx, filepath.Join(s.sourceDir, entry.Name()), entry.Name())
		if ok {
			records = append(records, record)
		}
	}
	sortRecords(records)
	return records, nil
}

// discoverManaged 是受托模式的插件发现：只生成有有效当前版本记录的已安装插件。
// 目录存在但没有当前版本记录视为未安装，不生成可执行记录。
func (s *system) discoverManaged(ctx context.Context) ([]pluginRecord, error) {
	entries, err := os.ReadDir(s.sourceDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, pluginReadFailed("failed to read system plugin program directory", err)
	}
	records := []pluginRecord{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, pluginReadFailed("read cancelled", err)
		}
		pluginID := entry.Name()
		directory := filepath.Join(s.sourceDir, pluginID)
		store, err := release.NewProgramStore(directory, types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: pluginID})
		if err != nil {
			continue
		}
		current, err := store.Current()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			records = append(records, unavailablePluginRecord(pluginID, directory, "当前插件版本记录不可读："+err.Error(), s.boxVersion))
			continue
		}
		record, ok := s.loadManagedRecord(ctx, current.ProgramDirectory, pluginID, current.Version)
		if ok {
			records = append(records, record)
		}
	}
	sortRecords(records)
	return records, nil
}

func sortRecords(records []pluginRecord) {
	sort.SliceStable(records, func(i int, j int) bool {
		return records[i].displayName() < records[j].displayName()
	})
}

func (s *system) loadRecord(ctx context.Context, directory string, sourceID string) (pluginRecord, bool) {
	manifestFile := filepath.Join(directory, "manifest.json")
	if !fileExists(manifestFile) {
		return pluginRecord{}, false
	}
	manifest, err := readJSONFile[types.SystemPluginManifest](ctx, manifestFile)
	if err != nil {
		reason := "系统插件声明无法读取：" + err.Error()
		return unavailablePluginRecord(sourceID, directory, reason, s.boxVersion), true
	}
	manifest = normalizeManifest(manifest)
	return s.buildRecord(ctx, manifest, sourceID, directory), true
}

// loadManagedRecord 从当前版本目录加载插件；版本必须与当前版本记录一致。
func (s *system) loadManagedRecord(ctx context.Context, directory string, sourceID string, expectedVersion string) (pluginRecord, bool) {
	manifestFile := filepath.Join(directory, "manifest.json")
	if !fileExists(manifestFile) {
		return unavailablePluginRecord(sourceID, directory, "系统插件版本目录缺少 manifest.json", s.boxVersion), true
	}
	manifest, err := readJSONFile[types.SystemPluginManifest](ctx, manifestFile)
	if err != nil {
		reason := "系统插件声明无法读取：" + err.Error()
		return unavailablePluginRecord(sourceID, directory, reason, s.boxVersion), true
	}
	manifest = normalizeManifest(manifest)
	if manifest.Version != strings.TrimSpace(expectedVersion) {
		return unavailablePluginRecord(sourceID, directory, "插件版本与当前版本记录不一致", s.boxVersion), true
	}
	return s.buildRecord(ctx, manifest, sourceID, directory), true
}

func (s *system) buildRecord(ctx context.Context, manifest types.SystemPluginManifest, sourceID string, directory string) pluginRecord {
	record := pluginRecord{
		manifest:      manifest,
		sourceID:      sourceID,
		directory:     directory,
		compatibility: release.AssessEucliBoxCompatibility(manifest.Version, s.boxVersion, manifest.EucliBoxCompatibility),
		status:        types.SystemPluginStatusActive,
		enabled:       true,
	}
	if err := validateManifestCore(manifest); err != nil {
		record.markUnavailable("系统插件声明无效：" + err.Error())
	}
	if !record.compatibility.Compatible {
		record.markUnavailable(record.compatibility.Reason)
	}
	if manifest.ID != "" {
		if dataDirectory, err := s.pluginDataDirectory(manifest.ID); err != nil {
			record.markUnavailable("系统插件资料位置无效：" + err.Error())
		} else {
			record.dataDirectory = dataDirectory
		}
	}
	defaultConfig, err := readMapFile(ctx, filepath.Join(directory, "config.json"))
	if err != nil {
		record.markUnavailable(err.Error())
	} else {
		record.defaultConfig = defaultConfig
	}
	if manifest.ID != "" {
		state, err := s.loadPluginState(ctx, manifest.ID)
		if err != nil {
			record.markUnavailable(err.Error())
		} else {
			record.enabled = state.Enabled
		}
		userConfig, err := s.loadUserConfig(ctx, manifest.ID)
		if err != nil {
			record.markUnavailable(err.Error())
		} else {
			record.userConfig = userConfig
		}
	}
	if executable, err := selectExecutable(directory, manifest.Binaries); err != nil {
		record.markUnavailable(err.Error())
	} else {
		record.executable = executable
	}
	return record
}

func normalizeManifest(manifest types.SystemPluginManifest) types.SystemPluginManifest {
	manifest.ID = strings.TrimSpace(manifest.ID)
	manifest.Name = strings.TrimSpace(manifest.Name)
	manifest.Description = strings.TrimSpace(manifest.Description)
	manifest.Version = strings.TrimSpace(manifest.Version)
	manifest.Hosting.Start = strings.TrimSpace(manifest.Hosting.Start)
	manifest.Hosting.Restart = strings.TrimSpace(manifest.Hosting.Restart)
	for index := range manifest.Binaries {
		manifest.Binaries[index].GOOS = strings.TrimSpace(manifest.Binaries[index].GOOS)
		manifest.Binaries[index].GOARCH = strings.TrimSpace(manifest.Binaries[index].GOARCH)
		manifest.Binaries[index].Path = strings.TrimSpace(manifest.Binaries[index].Path)
	}
	for capabilityIndex := range manifest.Capabilities {
		capability := &manifest.Capabilities[capabilityIndex]
		capability.Type = strings.TrimSpace(capability.Type)
		for interfaceIndex := range capability.Interfaces {
			item := &capability.Interfaces[interfaceIndex]
			item.ID = strings.TrimSpace(item.ID)
			item.DefaultName = strings.TrimSpace(item.DefaultName)
			item.Description = strings.TrimSpace(item.Description)
		}
	}
	return manifest
}

func validateManifestCore(manifest types.SystemPluginManifest) error {
	if manifest.ProtocolVersion != systemplugin.ProtocolVersion {
		return pluginInvalid("system plugin protocolVersion is not supported", nil)
	}
	if manifest.ID == "" {
		return pluginInvalid("system plugin manifest id is required", nil)
	}
	if manifest.Name == "" {
		return pluginInvalid("system plugin manifest name is required", nil)
	}
	if manifest.Description == "" {
		return pluginInvalid("system plugin manifest description is required", nil)
	}
	if manifest.Hosting.Start != types.SystemPluginStartBoot && manifest.Hosting.Start != types.SystemPluginStartLazy {
		return pluginInvalid("system plugin hosting start must be boot or lazy", nil)
	}
	if manifest.Hosting.Restart != types.SystemPluginRestartOnFailure && manifest.Hosting.Restart != types.SystemPluginRestartNever {
		return pluginInvalid("system plugin hosting restart must be on-failure or never", nil)
	}
	if manifest.Hosting.StopTimeoutMs <= 0 {
		return pluginInvalid("system plugin hosting stopTimeoutMs must be positive", nil)
	}
	if !manifest.Hosting.Resident && manifest.Hosting.Start != types.SystemPluginStartLazy {
		return pluginInvalid("system plugin hosting start must be lazy when the plugin is not resident", nil)
	}
	if len(manifest.Binaries) == 0 {
		return pluginInvalid("system plugin manifest must declare at least one binary", nil)
	}
	seenCapabilities := map[string]struct{}{}
	for _, capability := range manifest.Capabilities {
		if capability.Type == "" {
			return pluginInvalid("system plugin capability type is required", nil)
		}
		if _, ok := seenCapabilities[capability.Type]; ok {
			return pluginInvalid("system plugin capability type must be unique", nil)
		}
		seenCapabilities[capability.Type] = struct{}{}
		if capability.Type == systemplugin.CapabilityPlaceholderValues && len(capability.Interfaces) == 0 {
			return pluginInvalid("system plugin placeholder-values capability must declare at least one interface", nil)
		}
		seenInterfaces := map[string]struct{}{}
		for _, item := range capability.Interfaces {
			if item.ID == "" || item.DefaultName == "" {
				return pluginInvalid("system plugin capability interface id and defaultName are required", nil)
			}
			if _, ok := seenInterfaces[item.ID]; ok {
				return pluginInvalid("system plugin capability interface id must be unique", nil)
			}
			seenInterfaces[item.ID] = struct{}{}
		}
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
	return types.SystemPluginView{
		ID:                    r.manifest.ID,
		SourceID:              r.sourceID,
		Name:                  r.displayName(),
		Description:           r.manifest.Description,
		Version:               r.manifest.Version,
		EucliBoxCompatibility: r.manifest.EucliBoxCompatibility,
		Compatibility:         r.compatibility,
		Hosting:               r.manifest.Hosting,
		Status:                r.status,
		StatusMessage:         r.statusMessage,
		Installed:             true,
		CurrentVersion:        r.manifest.Version,
		Enabled:               r.enabled,
		DefaultConfig:         copyMap(r.defaultConfig),
		UserConfig:            copyMap(r.userConfig.UserConfig),
		ConfigSchema:          copyMap(r.manifest.ConfigSchema),
		PlaceholderInterfaces: r.interfaceViews(),
	}
}

// installStatusFor 把插件状态映射到统一安装状态词表。
func (s *system) installStatusFor(record pluginRecord) string {
	operationFile := filepath.Join(s.pluginProgramRoot(record.locatorID()), "operation.json")
	if recordValue, err := release.ReadOperationRecord(operationFile); err == nil && recordValue.Result == release.OperationResultFailed {
		return types.ArtifactStatusFailed
	}
	if record.status == types.SystemPluginStatusActive {
		return types.ArtifactStatusActive
	}
	return types.ArtifactStatusUnavailable
}

// managedPrograms 表示插件程序由外部程序根目录托管（受托运行模式）。
func (s *system) managedPrograms() bool {
	return s.programRoot != ""
}

func unavailablePluginRecord(sourceID string, directory string, reason string, boxVersion string) pluginRecord {
	return pluginRecord{
		manifest:      types.SystemPluginManifest{Description: "系统插件资料不可用"},
		sourceID:      sourceID,
		directory:     directory,
		compatibility: types.CompatibilityStatus{Reason: reason, CurrentEucliBoxVersion: boxVersion},
		status:        types.SystemPluginStatusUnavailable,
		statusMessage: reason,
		enabled:       true,
	}
}

func (r *pluginRecord) markUnavailable(reason string) {
	if r.status == types.SystemPluginStatusUnavailable {
		return
	}
	r.status = types.SystemPluginStatusUnavailable
	r.statusMessage = strings.TrimSpace(reason)
}

func (r pluginRecord) locatorID() string {
	if r.manifest.ID != "" {
		return r.manifest.ID
	}
	return r.sourceID
}

func (r pluginRecord) displayName() string {
	if r.manifest.Name != "" {
		return r.manifest.Name
	}
	return r.sourceID
}

// placeholderInterfaces 返回插件声明的占位符接口；未声明该能力时为空。
func (r pluginRecord) placeholderInterfaces() []types.SystemPluginPlaceholderInterface {
	return manifestPlaceholderInterfaces(r.manifest)
}

// manifestPlaceholderInterfaces 从清单能力中取占位符接口声明。
func manifestPlaceholderInterfaces(manifest types.SystemPluginManifest) []types.SystemPluginPlaceholderInterface {
	for _, capability := range manifest.Capabilities {
		if capability.Type == systemplugin.CapabilityPlaceholderValues {
			return capability.Interfaces
		}
	}
	return nil
}

// declaredPlaceholderInterface 返回插件声明的某个占位符接口。
func (r pluginRecord) declaredPlaceholderInterface(interfaceID string) (types.SystemPluginPlaceholderInterface, bool) {
	interfaceID = strings.TrimSpace(interfaceID)
	for _, item := range r.placeholderInterfaces() {
		if item.ID == interfaceID {
			return item, true
		}
	}
	return types.SystemPluginPlaceholderInterface{}, false
}

// wireCapabilities 把清单能力转换为握手声明：只交换类型与接口标识。
func (r pluginRecord) wireCapabilities() []systemplugin.Capability {
	out := make([]systemplugin.Capability, 0, len(r.manifest.Capabilities))
	for _, capability := range r.manifest.Capabilities {
		interfaces := make([]string, 0, len(capability.Interfaces))
		for _, item := range capability.Interfaces {
			interfaces = append(interfaces, item.ID)
		}
		out = append(out, systemplugin.Capability{Type: capability.Type, Interfaces: interfaces})
	}
	return out
}

func (r pluginRecord) interfaceViews() []types.SystemPluginPlaceholderInterfaceView {
	declared := r.placeholderInterfaces()
	out := make([]types.SystemPluginPlaceholderInterfaceView, 0, len(declared))
	for _, item := range declared {
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
