package releasechecksystem

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

type System interface {
	Snapshot() types.ReleaseCheckSnapshot
	Refresh(ctx context.Context, kind string) types.ReleaseCheckSnapshot
}

type NetworkSystem interface {
	Do(ctx context.Context, request types.HTTPRequest) (types.HTTPResponse, error)
}

type ToolSystem interface {
	ListTools(ctx context.Context) ([]types.ToolSummary, error)
}

type PluginSystem interface {
	ListPlugins(ctx context.Context) ([]types.SystemPluginSummary, error)
}

type Config struct {
	BoxVersion string
	APIBaseURL string
	IndexBase  string
	Now        func() time.Time
}

type checkRunner interface {
	CheckOnly(ctx context.Context, installed []releasecheck.InstalledArtifact, currentBoxVersion string, requested []types.ReleaseArtifactIdentity) types.ReleaseCheckSnapshot
}

type system struct {
	boxVersion string
	now        func() time.Time
	checker    checkRunner
	catalog    releasecatalog.Catalog
	tools      ToolSystem
	plugins    PluginSystem

	mu       sync.RWMutex
	snapshot types.ReleaseCheckSnapshot
	running  bool
}

func NewSystem(config Config, network NetworkSystem, tools ToolSystem, plugins PluginSystem) (System, error) {
	if network == nil {
		return nil, fmt.Errorf("发行检查需要网络请求能力")
	}
	if tools == nil {
		return nil, fmt.Errorf("发行检查需要工具状态能力")
	}
	if plugins == nil {
		return nil, fmt.Errorf("发行检查需要插件状态能力")
	}
	boxVersion := strings.TrimSpace(config.BoxVersion)
	if err := release.ValidateVersion(boxVersion); err != nil {
		return nil, fmt.Errorf("发行检查的业务端版本无效：%w", err)
	}
	checker, err := releasecheck.New(releasecheck.Config{Client: networkHTTPDoer{network: network}, APIBaseURL: config.APIBaseURL, IndexBase: config.IndexBase, Now: config.Now})
	if err != nil {
		return nil, err
	}
	return NewSystemWithChecker(config, checker, tools, plugins, boxVersion)
}

// NewSystemWithChecker 使用业务端统一创建的官方检查器；工具和插件安装系统共用同一份候选读取。
func NewSystemWithChecker(config Config, checker checkRunner, tools ToolSystem, plugins PluginSystem, boxVersion string) (System, error) {
	if checker == nil {
		return nil, fmt.Errorf("发行检查需要官方候选读取器")
	}
	if tools == nil {
		return nil, fmt.Errorf("发行检查需要工具状态能力")
	}
	if plugins == nil {
		return nil, fmt.Errorf("发行检查需要插件状态能力")
	}
	if err := release.ValidateVersion(boxVersion); err != nil {
		return nil, fmt.Errorf("发行检查的业务端版本无效：%w", err)
	}
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	catalog, err := releasecatalog.Load()
	if err != nil {
		return nil, err
	}
	return newSystem(config, checker, catalog, tools, plugins, boxVersion), nil
}

func newSystem(config Config, checker checkRunner, catalog releasecatalog.Catalog, tools ToolSystem, plugins PluginSystem, boxVersion string) *system {
	return &system{
		boxVersion: boxVersion,
		now:        config.Now,
		checker:    checker,
		catalog:    catalog,
		tools:      tools,
		plugins:    plugins,
		snapshot:   releasecheck.PendingSnapshot(),
	}
}

// Refresh 执行一次用户主动的分类刷新。kind 为空表示全量刷新；
// kind 只能是 eucli-box、tool 或 plugin，且只读取该分类对应官方仓库的一份统一版本索引。
func (s *system) Refresh(ctx context.Context, kind string) types.ReleaseCheckSnapshot {
	if ctx == nil {
		ctx = context.Background()
	}
	kind = strings.TrimSpace(kind)
	if kind != "" && !validRefreshKind(kind) {
		return failedKindSnapshot(s.now, fmt.Errorf("不支持的刷新分类 %q", kind))
	}
	s.mu.Lock()
	if s.running {
		snapshot := cloneSnapshot(s.snapshot)
		s.mu.Unlock()
		return snapshot
	}
	s.running = true
	previous := cloneSnapshot(s.snapshot)
	s.snapshot = releasecheck.CheckingSnapshot(previous, s.now())
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	requested := s.requestedArtifacts(kind)
	installed, localFailures := s.installedArtifacts(ctx)
	snapshot := s.checker.CheckOnly(ctx, installed, s.boxVersion, requested)
	applyLocalFailures(&snapshot, localFailures)
	snapshot = mergeKindSnapshot(previous, snapshot, kind)

	s.mu.Lock()
	s.snapshot = cloneSnapshot(snapshot)
	s.mu.Unlock()
	return snapshot
}

func (s *system) Snapshot() types.ReleaseCheckSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSnapshot(s.snapshot)
}

// requestedArtifacts 按刷新分类筛选正式白名单中的发布物。
func (s *system) requestedArtifacts(kind string) []types.ReleaseArtifactIdentity {
	requested := make([]types.ReleaseArtifactIdentity, 0, len(s.catalog.Artifacts))
	for _, artifact := range s.catalog.Artifacts {
		if kind != "" && artifact.Kind != kind {
			continue
		}
		requested = append(requested, artifact)
	}
	return requested
}

// mergeKindSnapshot 把本次分类刷新结果与上一次快照合并：
// 本次刷新的分类使用最新结果，其他分类保留上一次成功结果；全量刷新直接替换全部。
func mergeKindSnapshot(previous types.ReleaseCheckSnapshot, current types.ReleaseCheckSnapshot, kind string) types.ReleaseCheckSnapshot {
	if kind == "" {
		return current
	}
	results := make([]types.ReleaseCheckResult, 0, len(current.Results)+len(previous.Results))
	for _, result := range current.Results {
		results = append(results, result)
	}
	for _, result := range previous.Results {
		if result.Artifact.Kind == kind {
			continue
		}
		results = append(results, result)
	}
	sort.Slice(results, func(i int, j int) bool {
		if results[i].Artifact.Kind != results[j].Artifact.Kind {
			return results[i].Artifact.Kind < results[j].Artifact.Kind
		}
		return results[i].Artifact.ID < results[j].Artifact.ID
	})
	status := types.ReleaseCheckStatusCompleted
	for _, result := range results {
		if result.Status == types.ReleaseCheckStatusFailed {
			status = types.ReleaseCheckStatusFailed
			break
		}
	}
	return types.ReleaseCheckSnapshot{Status: status, StartedAt: current.StartedAt, CheckedAt: current.CheckedAt, Results: results}
}

func validRefreshKind(kind string) bool {
	switch kind {
	case types.ReleaseArtifactKindBox, types.ReleaseArtifactKindTool, types.ReleaseArtifactKindPlugin:
		return true
	default:
		return false
	}
}

func failedKindSnapshot(now func() time.Time, err error) types.ReleaseCheckSnapshot {
	started := now()
	return types.ReleaseCheckSnapshot{
		Status:        types.ReleaseCheckStatusFailed,
		StartedAt:     started,
		CheckedAt:     started,
		Results:       []types.ReleaseCheckResult{},
		FailureReason: strings.TrimSpace(err.Error()),
	}
}

func (s *system) installedArtifacts(ctx context.Context) ([]releasecheck.InstalledArtifact, map[string]string) {
	installed := []releasecheck.InstalledArtifact{{
		Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindBox, ID: types.ReleaseArtifactKindBox},
		Version:  s.boxVersion,
	}}
	failures := map[string]string{}
	tools, err := s.tools.ListTools(ctx)
	if err != nil {
		failures[types.ReleaseArtifactKindTool] = "读取当前工具状态失败：" + err.Error()
	} else {
		for _, tool := range tools {
			if tool.Status != types.ToolAvailabilityActive {
				continue
			}
			identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: strings.TrimSpace(tool.ID)}
			version := strings.TrimSpace(tool.Version)
			if identity.ID == "" || release.ValidateVersion(version) != nil {
				continue
			}
			if release.ValidateEucliBoxCompatibility(tool.EucliBoxCompatibility) != nil {
				continue
			}
			compatibility := tool.EucliBoxCompatibility
			installed = append(installed, releasecheck.InstalledArtifact{Artifact: identity, Version: version, Compatibility: &compatibility})
		}
	}
	plugins, err := s.plugins.ListPlugins(ctx)
	if err != nil {
		failures[types.ReleaseArtifactKindPlugin] = "读取当前插件状态失败：" + err.Error()
	} else {
		for _, plugin := range plugins {
			if !plugin.Installed {
				continue
			}
			identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: strings.TrimSpace(plugin.ID)}
			version := strings.TrimSpace(plugin.Version)
			if identity.ID == "" || release.ValidateVersion(version) != nil {
				continue
			}
			if release.ValidateEucliBoxCompatibility(plugin.EucliBoxCompatibility) != nil {
				continue
			}
			compatibility := plugin.EucliBoxCompatibility
			installed = append(installed, releasecheck.InstalledArtifact{Artifact: identity, Version: version, Compatibility: &compatibility})
		}
	}
	return installed, failures
}

func applyLocalFailures(snapshot *types.ReleaseCheckSnapshot, failures map[string]string) {
	if snapshot == nil || len(failures) == 0 {
		return
	}
	for index := range snapshot.Results {
		result := &snapshot.Results[index]
		if message := strings.TrimSpace(failures[result.Artifact.Kind]); message != "" {
			result.Status = types.ReleaseCheckStatusFailed
			result.FailureReason = message
			snapshot.Status = types.ReleaseCheckStatusFailed
		}
	}
}

func cloneSnapshot(source types.ReleaseCheckSnapshot) types.ReleaseCheckSnapshot {
	result := source
	result.Results = append([]types.ReleaseCheckResult(nil), source.Results...)
	for index := range result.Results {
		result.Results[index].AffectedArtifacts = append([]types.ReleaseArtifactIdentity(nil), source.Results[index].AffectedArtifacts...)
	}
	return result
}

type networkHTTPDoer struct {
	network NetworkSystem
}

func (d networkHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	headers := make(map[string]string, len(request.Header))
	for name, values := range request.Header {
		if len(values) > 0 {
			headers[name] = values[0]
		}
	}
	response, err := d.network.Do(request.Context(), types.HTTPRequest{Method: request.Method, URL: request.URL.String(), Headers: headers})
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: response.StatusCode,
		Header:     http.Header(response.Headers),
		Body:       io.NopCloser(bytes.NewReader(response.Body)),
		Request:    request,
	}, nil
}
