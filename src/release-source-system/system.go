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

// SourceState 是某类别安装来源的只读视图：当前选择、不可用原因、来源记录。
type SourceState interface {
	CurrentSource() string
	Problem() string
	Source(name string) (installsource.Source, bool)
}

// Config 是发行来源读取的装配事实；States 按发布物类别给出该类别的来源状态，
// 缺少某类别的状态时按始终使用官方源处理。
type Config struct {
	BoxVersion   string
	IndexBase    string
	RemoteClient releasecheck.HTTPDoer
	States       map[string]SourceState
}

type system struct {
	boxVersion   string
	checker      CandidateLister
	sources      releasecatalog.Sources
	remoteClient releasecheck.HTTPDoer
	tools        ToolSystem
	plugins      PluginSystem
	states       map[string]SourceState
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
		boxVersion:   boxVersion,
		checker:      checker,
		sources:      sources,
		remoteClient: config.RemoteClient,
		tools:        tools,
		plugins:      plugins,
		states:       config.States,
	}, nil
}

// ListInstallations 读取业务端当前真实的已装事实，不读取任何远端索引。
func (s *system) ListInstallations(ctx context.Context) (types.ArtifactInstallationList, error) {
	return types.ArtifactInstallationList{Artifacts: s.installedArtifacts(ctx)}, nil
}

// ListCandidates 按分类读取当前安装来源下的候选事实，并给出与已装事实的比对结论。
// 官方源按分类读取一次统一版本索引；货架来源按当前选择读取该货架，不读取官方索引。
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
	sourceName := s.currentSourceName(kind)
	switch {
	case s.sourceProblemText(kind) != "":
		candidates = []types.ArtifactReleaseCandidate{s.failedArtifactCandidate(types.ReleaseArtifactIdentity{Kind: kind, ID: kind}, installedByIdentity, "安装来源配置不可用："+s.sourceProblemText(kind))}
	case sourceName == "":
		candidates = []types.ArtifactReleaseCandidate{s.failedArtifactCandidate(types.ReleaseArtifactIdentity{Kind: kind, ID: kind}, installedByIdentity, "安装来源配置不可用")}
	case sourceName == types.ReleaseSourceOfficialName:
		candidates = s.officialCandidates(ctx, kind, installed, installedByIdentity)
	default:
		source, ok := s.currentSource(kind)
		if !ok {
			candidates = []types.ArtifactReleaseCandidate{s.failedArtifactCandidate(types.ReleaseArtifactIdentity{Kind: kind, ID: kind}, installedByIdentity, fmt.Sprintf("当前商店来源 %q 不是已注册来源", sourceName))}
			break
		}
		switch source.Kind {
		case types.ReleaseSourceKindShelf:
			candidates = s.shelfCandidates(ctx, kind, source, installed, installedByIdentity)
		case types.ReleaseSourceKindRemote:
			candidates = s.remoteCandidates(ctx, kind, source, installed, installedByIdentity)
		default:
			candidates = []types.ArtifactReleaseCandidate{s.failedArtifactCandidate(types.ReleaseArtifactIdentity{Kind: kind, ID: kind}, installedByIdentity, fmt.Sprintf("来源 %q 的提供方式 %q 无效", source.Name, source.Kind))}
		}
	}
	sort.Slice(candidates, func(i int, j int) bool {
		if candidates[i].Artifact.Kind != candidates[j].Artifact.Kind {
			return candidates[i].Artifact.Kind < candidates[j].Artifact.Kind
		}
		return candidates[i].Artifact.ID < candidates[j].Artifact.ID
	})
	return types.ArtifactCandidateList{SourceKind: s.currentSourceName(kind), Candidates: candidates}, nil
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

func (s *system) shelfCandidates(ctx context.Context, kind string, source installsource.Source, installed []types.ArtifactInstallation, installedByIdentity map[string]types.ArtifactInstallation) []types.ArtifactReleaseCandidate {
	reader, err := releasecheck.NewLocalSourceReader(source.Name, source.Path, kind)
	if err != nil {
		return []types.ArtifactReleaseCandidate{s.failedArtifactCandidate(types.ReleaseArtifactIdentity{Kind: kind, ID: kind}, installedByIdentity, fmt.Sprintf("货架 %q 不可用：%s", source.Name, err.Error()))}
	}
	items, err := reader.List(ctx)
	if err != nil {
		return []types.ArtifactReleaseCandidate{s.failedArtifactCandidate(types.ReleaseArtifactIdentity{Kind: kind, ID: kind}, installedByIdentity, fmt.Sprintf("读取货架 %q 失败：%s", source.Name, err.Error()))}
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
		candidates = append(candidates, s.installedOnlyCandidate(item, fmt.Sprintf("货架 %q 没有该发布物的候选成品", source.Name)))
	}
	return candidates
}

// remoteCandidates 读取网络来源的清单并给出候选；失败如实报告，不回退官方。
func (s *system) remoteCandidates(ctx context.Context, kind string, source installsource.Source, installed []types.ArtifactInstallation, installedByIdentity map[string]types.ArtifactInstallation) []types.ArtifactReleaseCandidate {
	if s.remoteClient == nil {
		return []types.ArtifactReleaseCandidate{s.failedArtifactCandidate(types.ReleaseArtifactIdentity{Kind: kind, ID: kind}, installedByIdentity, fmt.Sprintf("网络来源 %q 缺少网络请求能力", source.Name))}
	}
	reader, err := releasecheck.NewSourceReader(releasecheck.IndexSource{
		Name:     source.Name,
		Kind:     types.ReleaseSourceKindRemote,
		IndexURL: source.IndexURL,
	}, releasecheck.Config{Client: s.remoteClient})
	if err != nil {
		return []types.ArtifactReleaseCandidate{s.failedArtifactCandidate(types.ReleaseArtifactIdentity{Kind: kind, ID: kind}, installedByIdentity, fmt.Sprintf("网络来源 %q 不可用：%s", source.Name, err.Error()))}
	}
	records, err := reader.ListCandidates(ctx, kind)
	if err != nil {
		return []types.ArtifactReleaseCandidate{s.failedArtifactCandidate(types.ReleaseArtifactIdentity{Kind: kind, ID: kind}, installedByIdentity, fmt.Sprintf("读取网络来源 %q 失败：%s", source.Name, err.Error()))}
	}
	candidates := make([]types.ArtifactReleaseCandidate, 0, len(records))
	for _, record := range records {
		candidates = append(candidates, s.candidateFor(record, installedByIdentity))
	}
	for _, item := range installed {
		if item.Artifact.Kind != kind || hasCandidateFor(candidates, item.Artifact) {
			continue
		}
		candidates = append(candidates, s.installedOnlyCandidate(item, fmt.Sprintf("网络来源 %q 没有该发布物的候选成品", source.Name)))
	}
	return candidates
}

// currentSource 返回该类别的当前来源记录；未装配或未注册时返回 false。
func (s *system) currentSource(kind string) (installsource.Source, bool) {
	state := s.states[kind]
	if state == nil {
		return installsource.Source{}, false
	}
	return state.Source(s.currentSourceName(kind))
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
		Source:        source.Source,
		LatestVersion: source.Version,
		Status:        types.ReleaseCandidateStatusCompleted,
		PublishedAt:   source.PublishedAt,
		ReleaseURL:    source.ReleaseURL,
		ReleaseNotes:  source.ReleaseNotes,
		DownloadSize:  source.SizeBytes,
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
		Source:         s.currentSourceRef(installed.Artifact.Kind),
		Installed:      true,
		CurrentVersion: installed.Version,
		Status:         types.ReleaseCandidateStatusFailed,
		FailureReason:  reason,
	}
	return candidate
}

// failedArtifactCandidate 产生一个读取失败的候选项；若该发布物已装则保留其已装事实。
func (s *system) failedArtifactCandidate(artifact types.ReleaseArtifactIdentity, installedByIdentity map[string]types.ArtifactInstallation, reason string) types.ArtifactReleaseCandidate {
	candidate := types.ArtifactReleaseCandidate{
		Artifact:      artifact,
		Source:        s.currentSourceRef(artifact.Kind),
		Status:        types.ReleaseCandidateStatusFailed,
		FailureReason: reason,
	}
	if installed, ok := installedByIdentity[identityKey(artifact)]; ok {
		candidate.Installed = true
		candidate.CurrentVersion = installed.Version
	}
	return candidate
}

// currentSourceRef 返回该类别当前来源的对外描述；配置不可用或未注册时只保留名字。
func (s *system) currentSourceRef(kind string) types.ReleaseSourceRef {
	name := s.currentSourceName(kind)
	if name == "" {
		return types.ReleaseSourceRef{}
	}
	if name == types.ReleaseSourceOfficialName {
		official, err := s.sources.SourceFor(kind)
		if err != nil {
			return types.ReleaseSourceRef{Name: name, Kind: types.ReleaseSourceKindGithub}
		}
		return types.ReleaseSourceRef{Name: name, Kind: types.ReleaseSourceKindGithub, Repository: official.Repository, Ref: official.Ref}
	}
	source, ok := s.currentSource(kind)
	if !ok {
		return types.ReleaseSourceRef{Name: name}
	}
	reference := types.ReleaseSourceRef{Name: source.Name, Kind: source.Kind}
	if source.Kind == types.ReleaseSourceKindRemote {
		reference.IndexURL = source.IndexURL
	}
	return reference
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

// currentSourceName 返回该类别的当前来源名字；未装配时按官方源处理，空字符串表示来源配置不可用。
func (s *system) currentSourceName(kind string) string {
	state := s.states[kind]
	if state == nil {
		return installsource.OfficialSource
	}
	return strings.TrimSpace(state.CurrentSource())
}

// sourceProblemText 返回该类别来源配置不可用的原因；空字符串表示配置正常。
func (s *system) sourceProblemText(kind string) string {
	state := s.states[kind]
	if state == nil {
		return ""
	}
	return strings.TrimSpace(state.Problem())
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
