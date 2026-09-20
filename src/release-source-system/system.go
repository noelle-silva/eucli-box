package releasesourcesystem

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"eucli-box/pkg/installsource"
	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

// System 是业务端对发行来源的只读窄接口：按需读取已装事实与候选事实，自身不保存任何结果。
type System interface {
	ListInstallations(ctx context.Context) (types.ArtifactInstallationList, error)
	ListCandidates(ctx context.Context, kind string) (types.ArtifactCandidateList, error)
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

// CandidateLister 是官方统一版本索引的按分类读取能力。
type CandidateLister interface {
	ListCandidates(ctx context.Context, kind string) ([]releasecheck.CandidateRecord, error)
}

// Config 是发行来源读取的装配事实；CurrentSource 为 nil 表示始终使用官方源。
type Config struct {
	BoxVersion    string
	IndexBase     string
	CurrentSource func() installsource.Kind
	LocalSource   releasecheck.LocalShelf
}

type system struct {
	boxVersion    string
	checker       CandidateLister
	sources       releasecatalog.Sources
	tools         ToolSystem
	plugins       PluginSystem
	currentSource func() installsource.Kind
	localSource   releasecheck.LocalShelf
}

// NewSystem 创建发行来源读取系统：装配官方候选读取器，并持有工具与插件状态读取能力。
func NewSystem(config Config, network NetworkSystem, tools ToolSystem, plugins PluginSystem) (System, error) {
	checker, err := releasecheck.New(releasecheck.Config{Client: networkHTTPDoer{network: network}, IndexBase: config.IndexBase})
	if err != nil {
		return nil, err
	}
	return NewSystemWithChecker(config, checker, tools, plugins)
}

// NewSystemWithChecker 使用业务端统一创建的官方候选读取器。
func NewSystemWithChecker(config Config, checker CandidateLister, tools ToolSystem, plugins PluginSystem) (System, error) {
	if checker == nil {
		return nil, fmt.Errorf("发行来源读取需要官方候选读取器")
	}
	if tools == nil {
		return nil, fmt.Errorf("发行来源读取需要工具状态能力")
	}
	if plugins == nil {
		return nil, fmt.Errorf("发行来源读取需要插件状态能力")
	}
	boxVersion := strings.TrimSpace(config.BoxVersion)
	if err := release.ValidateVersion(boxVersion); err != nil {
		return nil, fmt.Errorf("发行来源读取的业务端版本无效：%w", err)
	}
	sources, err := releasecatalog.LoadSources()
	if err != nil {
		return nil, err
	}
	return &system{
		boxVersion:    boxVersion,
		checker:       checker,
		sources:       sources,
		tools:         tools,
		plugins:       plugins,
		currentSource: config.CurrentSource,
		localSource:   config.LocalSource,
	}, nil
}

// ListInstallations 读取业务端当前真实的已装事实，不读取任何远端索引。
func (s *system) ListInstallations(ctx context.Context) (types.ArtifactInstallationList, error) {
	return types.ArtifactInstallationList{Artifacts: s.installedArtifacts(ctx)}, nil
}

// ListCandidates 按分类读取当前安装来源下的候选事实，并给出与已装事实的比对结论。
// 官方源按分类读取一次统一版本索引；本地源读取本地商店货架，不读取官方索引。
func (s *system) ListCandidates(ctx context.Context, kind string) (types.ArtifactCandidateList, error) {
	kind = strings.TrimSpace(kind)
	if !s.supportsKind(kind) {
		return types.ArtifactCandidateList{}, fmt.Errorf("不支持的候选分类 %q", kind)
	}
	installed := s.installedArtifacts(ctx)
	installedByIdentity := make(map[string]types.ArtifactInstallation, len(installed))
	for _, item := range installed {
		installedByIdentity[identityKey(item.Artifact)] = item
	}

	var candidates []types.ArtifactReleaseCandidate
	if s.localMode() {
		candidates = s.localCandidates(ctx, kind, installed, installedByIdentity)
	} else {
		candidates = s.officialCandidates(ctx, kind, installed, installedByIdentity)
	}
	sort.Slice(candidates, func(i int, j int) bool {
		if candidates[i].Artifact.Kind != candidates[j].Artifact.Kind {
			return candidates[i].Artifact.Kind < candidates[j].Artifact.Kind
		}
		return candidates[i].Artifact.ID < candidates[j].Artifact.ID
	})
	return types.ArtifactCandidateList{SourceKind: s.sourceKind(), Candidates: candidates}, nil
}

// supportsKind 判断分类在当前来源下是否可读；本体不参与发行候选。
func (s *system) supportsKind(kind string) bool {
	return kind == types.ReleaseArtifactKindTool || kind == types.ReleaseArtifactKindPlugin
}

func (s *system) officialCandidates(ctx context.Context, kind string, installed []types.ArtifactInstallation, installedByIdentity map[string]types.ArtifactInstallation) []types.ArtifactReleaseCandidate {
	records, err := s.checker.ListCandidates(ctx, kind)
	if err != nil {
		return []types.ArtifactReleaseCandidate{s.failedArtifactCandidate(types.ReleaseArtifactIdentity{Kind: kind, ID: kind}, installedByIdentity, err.Error())}
	}
	candidates := make([]types.ArtifactReleaseCandidate, 0, len(records))
	for _, record := range records {
		candidates = append(candidates, s.candidateFor(record, installedByIdentity))
	}
	return candidates
}

func (s *system) localCandidates(ctx context.Context, kind string, installed []types.ArtifactInstallation, installedByIdentity map[string]types.ArtifactInstallation) []types.ArtifactReleaseCandidate {
	if s.localSource == nil {
		return []types.ArtifactReleaseCandidate{s.failedArtifactCandidate(types.ReleaseArtifactIdentity{Kind: kind, ID: kind}, installedByIdentity, "本地商店未激活")}
	}
	items, err := s.localSource.List(ctx)
	if err != nil {
		return []types.ArtifactReleaseCandidate{s.failedArtifactCandidate(types.ReleaseArtifactIdentity{Kind: kind, ID: kind}, installedByIdentity, "读取本地商店货架失败："+err.Error())}
	}
	candidates := make([]types.ArtifactReleaseCandidate, 0)
	for _, item := range items {
		if item.Candidate == nil || item.Artifact.Kind != kind {
			continue
		}
		candidates = append(candidates, s.candidateFromReleaseCandidate(item.Artifact, item.Candidate, installedByIdentity))
	}
	for _, item := range installed {
		if item.Artifact.Kind != kind || hasCandidateFor(candidates, item.Artifact) {
			continue
		}
		candidates = append(candidates, s.installedOnlyCandidate(item, "本地商店货架没有该发布物的候选成品"))
	}
	return candidates
}

func (s *system) candidateFor(record releasecheck.CandidateRecord, installedByIdentity map[string]types.ArtifactInstallation) types.ArtifactReleaseCandidate {
	if record.Candidate == nil {
		return s.failedArtifactCandidate(record.Artifact, installedByIdentity, record.FailureReason)
	}
	return s.candidateFromReleaseCandidate(record.Artifact, record.Candidate, installedByIdentity)
}

func (s *system) candidateFromReleaseCandidate(artifact types.ReleaseArtifactIdentity, source *releasecheck.ReleaseCandidate, installedByIdentity map[string]types.ArtifactInstallation) types.ArtifactReleaseCandidate {
	candidate := types.ArtifactReleaseCandidate{
		Artifact:      artifact,
		LatestVersion: source.Version,
		Status:        types.ReleaseCandidateStatusCompleted,
		PublishedAt:   source.PublishedAt,
		ReleaseURL:    source.ReleaseURL,
		ReleaseNotes:  source.ReleaseNotes,
		DownloadSize:  source.SizeBytes,
	}
	if official, err := s.sources.SourceFor(artifact.Kind); err == nil {
		candidate.Source = official
	}
	if installed, ok := installedByIdentity[identityKey(artifact)]; ok {
		candidate.Installed = true
		candidate.CurrentVersion = installed.Version
		if order, err := release.CompareVersions(source.Version, installed.Version); err == nil {
			candidate.UpdateAvailable = order > 0
		} else {
			candidate.Status = types.ReleaseCandidateStatusFailed
			candidate.FailureReason = err.Error()
		}
	} else {
		candidate.UpdateAvailable = true
	}
	if source.Compatibility != nil && strings.TrimSpace(s.boxVersion) != "" {
		status := release.AssessEucliBoxCompatibility(source.Version, s.boxVersion, *source.Compatibility)
		candidate.Compatibility = &status
	}
	return candidate
}

// installedOnlyCandidate 为「已装但在当前来源没有候选」的发布物保留已装事实并说明原因。
func (s *system) installedOnlyCandidate(installed types.ArtifactInstallation, reason string) types.ArtifactReleaseCandidate {
	candidate := types.ArtifactReleaseCandidate{
		Artifact:       installed.Artifact,
		Installed:      true,
		CurrentVersion: installed.Version,
		Status:         types.ReleaseCandidateStatusFailed,
		FailureReason:  reason,
	}
	if official, err := s.sources.SourceFor(installed.Artifact.Kind); err == nil {
		candidate.Source = official
	}
	return candidate
}

// failedArtifactCandidate 产生一个读取失败的候选项；若该发布物已装则保留其已装事实。
func (s *system) failedArtifactCandidate(artifact types.ReleaseArtifactIdentity, installedByIdentity map[string]types.ArtifactInstallation, reason string) types.ArtifactReleaseCandidate {
	candidate := types.ArtifactReleaseCandidate{
		Artifact:      artifact,
		Status:        types.ReleaseCandidateStatusFailed,
		FailureReason: reason,
	}
	if installed, ok := installedByIdentity[identityKey(artifact)]; ok {
		candidate.Installed = true
		candidate.CurrentVersion = installed.Version
	}
	if official, err := s.sources.SourceFor(artifact.Kind); err == nil {
		candidate.Source = official
	}
	return candidate
}

// installedArtifacts 读取可用工具与已装插件的真实版本事实。
func (s *system) installedArtifacts(ctx context.Context) []types.ArtifactInstallation {
	installed := make([]types.ArtifactInstallation, 0)
	tools, err := s.tools.ListTools(ctx)
	if err != nil {
		installed = append(installed, failedInstallation(types.ReleaseArtifactKindTool, "读取当前工具状态失败："+err.Error()))
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
			installed = append(installed, types.ArtifactInstallation{Artifact: identity, Version: version, Compatibility: &compatibility})
		}
	}
	plugins, err := s.plugins.ListPlugins(ctx)
	if err != nil {
		installed = append(installed, failedInstallation(types.ReleaseArtifactKindPlugin, "读取当前插件状态失败："+err.Error()))
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
			installed = append(installed, types.ArtifactInstallation{Artifact: identity, Version: version, Compatibility: &compatibility})
		}
	}
	sort.Slice(installed, func(i int, j int) bool {
		if installed[i].Artifact.Kind != installed[j].Artifact.Kind {
			return installed[i].Artifact.Kind < installed[j].Artifact.Kind
		}
		return installed[i].Artifact.ID < installed[j].Artifact.ID
	})
	return installed
}

func (s *system) localMode() bool {
	return s.currentSource != nil && s.currentSource() == installsource.KindLocal
}

func (s *system) sourceKind() string {
	if s.localMode() {
		return string(installsource.KindLocal)
	}
	return string(installsource.KindOfficial)
}

func failedInstallation(kind string, reason string) types.ArtifactInstallation {
	return types.ArtifactInstallation{Artifact: types.ReleaseArtifactIdentity{Kind: kind, ID: kind}, FailureReason: reason}
}

func hasCandidateFor(candidates []types.ArtifactReleaseCandidate, artifact types.ReleaseArtifactIdentity) bool {
	for _, candidate := range candidates {
		if candidate.Artifact == artifact {
			return true
		}
	}
	return false
}

func identityKey(identity types.ReleaseArtifactIdentity) string {
	return identity.Kind + ":" + identity.ID
}

type networkHTTPDoer struct {
	network NetworkSystem
}

func (d networkHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	if d.network == nil {
		return http.DefaultClient.Do(request)
	}
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
