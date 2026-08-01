package releasechecksystem

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

const defaultCheckInterval = 24 * time.Hour

type System interface {
	Start(ctx context.Context) error
	Snapshot() types.ReleaseCheckSnapshot
	Refresh(ctx context.Context) types.ReleaseCheckSnapshot
	Shutdown(ctx context.Context) error
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
	Interval   time.Duration
	APIBaseURL string
	Now        func() time.Time
}

type checkRunner interface {
	Check(ctx context.Context, installed []releasecheck.InstalledArtifact, currentBoxVersion string) types.ReleaseCheckSnapshot
}

type system struct {
	boxVersion string
	interval   time.Duration
	now        func() time.Time
	checker    checkRunner
	tools      ToolSystem
	plugins    PluginSystem

	mu       sync.RWMutex
	snapshot types.ReleaseCheckSnapshot
	running  bool
	started  bool
	cancel   context.CancelFunc
	wait     sync.WaitGroup
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
	if config.Interval == 0 {
		config.Interval = defaultCheckInterval
	}
	if config.Interval < 0 {
		return nil, fmt.Errorf("发行检查间隔不能为负数")
	}
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	checker, err := releasecheck.New(releasecheck.Config{Client: networkHTTPDoer{network: network}, APIBaseURL: config.APIBaseURL, Now: config.Now})
	if err != nil {
		return nil, err
	}
	return newSystem(config, checker, tools, plugins, boxVersion), nil
}

func newSystem(config Config, checker checkRunner, tools ToolSystem, plugins PluginSystem, boxVersion string) *system {
	return &system{
		boxVersion: boxVersion,
		interval:   config.Interval,
		now:        config.Now,
		checker:    checker,
		tools:      tools,
		plugins:    plugins,
		snapshot:   releasecheck.PendingSnapshot(),
	}
}

func (s *system) Start(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("发行检查启动上下文不能为空")
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("发行检查已经启动")
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.started = true
	s.cancel = cancel
	s.wait.Add(1)
	s.mu.Unlock()
	go s.loop(runCtx)
	return nil
}

func (s *system) loop(ctx context.Context) {
	defer s.wait.Done()
	s.Refresh(ctx)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Refresh(ctx)
		}
	}
}

func (s *system) Refresh(ctx context.Context) types.ReleaseCheckSnapshot {
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	if s.running {
		snapshot := cloneSnapshot(s.snapshot)
		s.mu.Unlock()
		return snapshot
	}
	s.running = true
	s.snapshot = releasecheck.CheckingSnapshot(s.snapshot, s.now())
	s.mu.Unlock()

	installed, localFailures := s.installedArtifacts(ctx)
	snapshot := s.checker.Check(ctx, installed, s.boxVersion)
	applyLocalFailures(&snapshot, localFailures)

	s.mu.Lock()
	s.running = false
	s.snapshot = cloneSnapshot(snapshot)
	s.mu.Unlock()
	return snapshot
}

func (s *system) Snapshot() types.ReleaseCheckSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSnapshot(s.snapshot)
}

func (s *system) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("发行检查停止上下文不能为空")
	}
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		s.wait.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return fmt.Errorf("等待发行检查停止失败：%w", ctx.Err())
	case <-done:
		return nil
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
			identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: strings.TrimSpace(tool.ID)}
			version := strings.TrimSpace(tool.Version)
			if identity.ID == "" || release.ValidateVersion(version) != nil {
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
			identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: strings.TrimSpace(plugin.ID)}
			version := strings.TrimSpace(plugin.Version)
			if identity.ID == "" || release.ValidateVersion(version) != nil {
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
