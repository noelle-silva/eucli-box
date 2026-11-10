package installsource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

// OfficialSource 是内嵌官方发行来源的保留字；不允许作为自定义来源名字。
const OfficialSource = types.ReleaseSourceOfficialName

// 来源提供方式：本机货架读取目录，网络商店读取静态清单。
const (
	SourceKindShelf  = types.ReleaseSourceKindShelf
	SourceKindRemote = types.ReleaseSourceKindRemote
)

// Source 是用户注册的一条商店来源：名字是对外身份，提供方式决定读取位置。
type Source struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Path     string `json:"path,omitempty"`
	IndexURL string `json:"indexUrl,omitempty"`
}

// Shelf 是本机来源的兼容视图：旧客户端只认名字与路径。
type Shelf struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Config 是安装来源配置的持久化形态：当前选择 + 来源注册表。
// Source 取值为 official 或某个已注册来源名字；注册表按注册顺序保存。
type Config struct {
	Source  string   `json:"source"`
	Sources []Source `json:"sources"`
}

// UnmarshalJSON 读取来源配置：新格式 sources；旧格式 shelves 自动升格为本机来源。
// 两种格式同时出现时无法确定唯一事实，直接拒绝。
func (c *Config) UnmarshalJSON(payload []byte) error {
	var document struct {
		Source  string   `json:"source"`
		Sources []Source `json:"sources"`
		Shelves []Shelf  `json:"shelves"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return err
	}
	if len(document.Sources) > 0 && len(document.Shelves) > 0 {
		return fmt.Errorf("来源配置同时包含 sources 与 shelves，无法确定唯一事实")
	}
	sources := document.Sources
	if len(document.Shelves) > 0 {
		sources = make([]Source, 0, len(document.Shelves))
		for _, shelf := range document.Shelves {
			sources = append(sources, Source{Name: shelf.Name, Kind: SourceKindShelf, Path: shelf.Path})
		}
	}
	*c = Config{Source: document.Source, Sources: sources}
	return nil
}

// DefaultConfig 是配置文件缺失时的初始形态：空注册表 + 官方源。
func DefaultConfig() Config {
	return Config{Source: OfficialSource, Sources: []Source{}}
}

// Clone 深拷贝配置，避免调用方与状态共享底层切片。
func (c Config) Clone() Config {
	sources := make([]Source, len(c.Sources))
	copy(sources, c.Sources)
	return Config{Source: c.Source, Sources: sources}
}

// normalizeConfig 裁剪并校验配置，返回可保存的规范形态。
func normalizeConfig(config Config) (Config, error) {
	source := strings.TrimSpace(config.Source)
	if source == "" {
		return Config{}, fmt.Errorf("安装来源配置缺少来源选择")
	}
	sources := make([]Source, 0, len(config.Sources))
	seen := make(map[string]struct{}, len(config.Sources))
	for _, item := range config.Sources {
		normalized, err := normalizeSource(item)
		if err != nil {
			return Config{}, err
		}
		if _, ok := seen[normalized.Name]; ok {
			return Config{}, fmt.Errorf("来源名字 %q 重复", normalized.Name)
		}
		seen[normalized.Name] = struct{}{}
		sources = append(sources, normalized)
	}
	if source != OfficialSource {
		if _, ok := seen[source]; !ok {
			return Config{}, fmt.Errorf("安装来源 %q 不是已注册来源", source)
		}
	}
	return Config{Source: source, Sources: sources}, nil
}

// normalizeSource 校验单条来源：名字是身份，提供方式决定哪些位置字段有意义。
func normalizeSource(item Source) (Source, error) {
	name := strings.TrimSpace(item.Name)
	if name == "" {
		return Source{}, fmt.Errorf("来源名字不能为空")
	}
	if strings.EqualFold(name, OfficialSource) {
		return Source{}, fmt.Errorf("来源名字 %q 是保留字，不能使用", name)
	}
	kind := strings.TrimSpace(item.Kind)
	if kind == "" {
		kind = SourceKindShelf
	}
	path := strings.TrimSpace(item.Path)
	indexURL := strings.TrimSpace(item.IndexURL)
	switch kind {
	case SourceKindShelf:
		if path == "" {
			return Source{}, fmt.Errorf("本机来源 %q 缺少路径", name)
		}
		if indexURL != "" {
			return Source{}, fmt.Errorf("本机来源 %q 不能填写网络地址", name)
		}
		return Source{Name: name, Kind: kind, Path: path}, nil
	case SourceKindRemote:
		if path != "" {
			return Source{}, fmt.Errorf("网络来源 %q 不能填写本机路径", name)
		}
		normalized, err := normalizeIndexURL(indexURL)
		if err != nil {
			return Source{}, fmt.Errorf("网络来源 %q 的清单地址无效：%w", name, err)
		}
		return Source{Name: name, Kind: kind, IndexURL: normalized}, nil
	default:
		return Source{}, fmt.Errorf("来源 %q 的提供方式 %q 无效", name, kind)
	}
}

// normalizeIndexURL 校验网络清单地址：固定 HTTPS；隔离验证允许回环主机，
// 且不允许账号信息、查询参数与锚点。
func normalizeIndexURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if err := releasecatalog.ValidateSourceAddress(value); err != nil {
		return "", err
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("不能包含查询参数或锚点")
	}
	return parsed.String(), nil
}

// Store 是安装来源配置的持久化接口，由 data-storage-system 实现。
type Store interface {
	LoadInstallSource(ctx context.Context) (Config, error)
	SaveInstallSource(ctx context.Context, config Config) error
}

// State 是来源注册表与当前选择的唯一事实源：内存值 + 持久化，一次操作一次原子保存。
// 初始配置非法时进入「配置不可用」状态：本体照常启动，来源读取如实报错；
// 重新设置官方源或注册一个来源即完成配置重建。
type State struct {
	mu      sync.RWMutex
	config  Config
	problem error
	store   Store
}

// NewState 构造来源状态；initial 非法时进入「配置不可用」状态，不阻断本体启动。
func NewState(initial Config, store Store) *State {
	normalized, err := normalizeConfig(initial)
	if err != nil {
		return &State{config: Config{Sources: []Source{}}, problem: fmt.Errorf("安装来源配置无效：%w", err), store: store}
	}
	return &State{config: normalized, store: store}
}

// NewStateUnavailable 构造「配置不可用」状态：加载失败的原因作为待修复原因保留。
func NewStateUnavailable(problem error, store Store) *State {
	if problem == nil {
		problem = fmt.Errorf("安装来源配置不可用")
	}
	return &State{config: Config{Sources: []Source{}}, problem: problem, store: store}
}

// Problem 返回配置不可用的原因；空字符串表示配置正常。
func (s *State) Problem() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.problem == nil {
		return ""
	}
	return s.problem.Error()
}

// CurrentSource 返回当前来源选择；配置不可用时返回空字符串。
func (s *State) CurrentSource() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.problem != nil {
		return ""
	}
	return s.config.Source
}

// Sources 返回按注册顺序排列的来源注册表副本；配置不可用时返回空表。
func (s *State) Sources() []Source {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.problem != nil {
		return []Source{}
	}
	return s.config.Clone().Sources
}

// Source 按名字读取来源记录；配置不可用时一律不存在。
func (s *State) Source(name string) (Source, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.problem != nil {
		return Source{}, false
	}
	target := strings.TrimSpace(name)
	for _, item := range s.config.Sources {
		if item.Name == target {
			return item, true
		}
	}
	return Source{}, false
}

// SourcePath 返回本机来源的目录；网络来源与不存在时返回 false。
func (s *State) SourcePath(name string) (string, bool) {
	source, ok := s.Source(name)
	if !ok || source.Kind != SourceKindShelf {
		return "", false
	}
	return source.Path, true
}

// Shelves 返回本机来源的兼容视图。
func (s *State) Shelves() []Shelf {
	sources := s.Sources()
	shelves := make([]Shelf, 0, len(sources))
	for _, item := range sources {
		if item.Kind == SourceKindShelf {
			shelves = append(shelves, Shelf{Name: item.Name, Path: item.Path})
		}
	}
	return shelves
}

// Shelf 按名字读取本机来源；网络来源一律不存在。
func (s *State) Shelf(name string) (Shelf, bool) {
	path, ok := s.SourcePath(name)
	if !ok {
		return Shelf{}, false
	}
	source, _ := s.Source(name)
	return Shelf{Name: source.Name, Path: path}, true
}

// ShelfPath 按名字读取本机来源路径；网络来源与不存在时返回 false。
func (s *State) ShelfPath(name string) (string, bool) {
	return s.SourcePath(name)
}

// commit 在锁内执行一次读改写：先校验，再持久化，成功后更新内存值并清除问题态。
func (s *State) commit(ctx context.Context, change func(current Config, problem error) (Config, error)) (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next, err := change(s.config.Clone(), s.problem)
	if err != nil {
		return s.config, err
	}
	next, err = normalizeConfig(next)
	if err != nil {
		return s.config, err
	}
	if s.store != nil {
		if err := s.store.SaveInstallSource(ctx, next); err != nil {
			return s.config, err
		}
	}
	s.config = next
	s.problem = nil
	return next, nil
}

// SetSource 切换来源：必须是官方保留字或已注册来源，其余拒绝。
// 配置不可用时只有官方保留字可以完成重建。
func (s *State) SetSource(ctx context.Context, source string) (string, error) {
	next, err := s.commit(ctx, func(current Config, problem error) (Config, error) {
		value := strings.TrimSpace(source)
		if problem != nil {
			if value != OfficialSource {
				return current, fmt.Errorf("安装来源配置不可用（%v），请先重新设置官方源或注册来源", problem)
			}
			return Config{Source: OfficialSource, Sources: []Source{}}, nil
		}
		current.Source = value
		return current, nil
	})
	return next.Source, err
}

// AddSource 注册来源；重名、保留字、位置缺失一律拒绝。
// 配置不可用时以新来源重建注册表，来源回到官方源。
func (s *State) AddSource(ctx context.Context, source Source) ([]Source, error) {
	next, err := s.commit(ctx, func(current Config, problem error) (Config, error) {
		if problem != nil {
			return Config{Source: OfficialSource, Sources: []Source{source}}, nil
		}
		current.Sources = append(current.Sources, source)
		return current, nil
	})
	return next.Sources, err
}

// SourcePatch 是一次来源修改的补丁：只提交要修改的字段。
type SourcePatch struct {
	NewName     *string
	NewKind     *string
	NewPath     *string
	NewIndexURL *string
}

// empty 判断补丁是否没有任何修改。
func (p SourcePatch) empty() bool {
	return p.NewName == nil && p.NewKind == nil && p.NewPath == nil && p.NewIndexURL == nil
}

// UpdateSource 改名 / 改提供方式 / 改位置；选中来源改名时选择跟随新名字。
// 提供方式变更时，不适用的一侧字段自动清空；其余冲突由结构校验拒绝。
func (s *State) UpdateSource(ctx context.Context, name string, patch SourcePatch) ([]Source, error) {
	if patch.empty() {
		return s.Sources(), fmt.Errorf("改来源必须提供新名字、新提供方式或新位置")
	}
	next, err := s.commit(ctx, func(current Config, problem error) (Config, error) {
		if problem != nil {
			return current, fmt.Errorf("安装来源配置不可用（%v），无法修改来源", problem)
		}
		target := strings.TrimSpace(name)
		for index := range current.Sources {
			item := &current.Sources[index]
			if item.Name != target {
				continue
			}
			previousKind := item.Kind
			if patch.NewName != nil {
				item.Name = strings.TrimSpace(*patch.NewName)
			}
			if patch.NewKind != nil {
				item.Kind = strings.TrimSpace(*patch.NewKind)
			}
			if patch.NewPath != nil {
				item.Path = strings.TrimSpace(*patch.NewPath)
			}
			if patch.NewIndexURL != nil {
				item.IndexURL = strings.TrimSpace(*patch.NewIndexURL)
			}
			if item.Kind != previousKind {
				switch item.Kind {
				case SourceKindShelf:
					item.IndexURL = ""
				case SourceKindRemote:
					item.Path = ""
				}
			}
			if current.Source == target {
				current.Source = item.Name
			}
			return current, nil
		}
		return current, fmt.Errorf("来源 %q 不存在", target)
	})
	return next.Sources, err
}

// RemoveSource 删除来源；删除已选中的来源时选择回落官方源。
func (s *State) RemoveSource(ctx context.Context, name string) ([]Source, error) {
	next, err := s.commit(ctx, func(current Config, problem error) (Config, error) {
		if problem != nil {
			return current, fmt.Errorf("安装来源配置不可用（%v），无法删除来源", problem)
		}
		target := strings.TrimSpace(name)
		kept := make([]Source, 0, len(current.Sources))
		found := false
		for _, item := range current.Sources {
			if item.Name == target {
				found = true
				continue
			}
			kept = append(kept, item)
		}
		if !found {
			return current, fmt.Errorf("来源 %q 不存在", target)
		}
		current.Sources = kept
		if current.Source == target {
			current.Source = OfficialSource
		}
		return current, nil
	})
	return next.Sources, err
}

// AddShelf 注册本机来源；等价于注册一条本机提供方式的来源。
func (s *State) AddShelf(ctx context.Context, name string, path string) ([]Shelf, error) {
	_, err := s.AddSource(ctx, Source{Name: name, Kind: SourceKindShelf, Path: path})
	return s.Shelves(), err
}

// UpdateShelf 改名 / 改路径；只作用于本机来源。
func (s *State) UpdateShelf(ctx context.Context, name string, newName *string, newPath *string) ([]Shelf, error) {
	if newName == nil && newPath == nil {
		return s.Shelves(), fmt.Errorf("改货架必须提供新名字或新路径")
	}
	if _, ok := s.Shelf(name); !ok {
		return s.Shelves(), fmt.Errorf("货架 %q 不存在", strings.TrimSpace(name))
	}
	_, err := s.UpdateSource(ctx, name, SourcePatch{NewName: newName, NewPath: newPath})
	return s.Shelves(), err
}

// RemoveShelf 删除本机来源；只作用于本机来源。
func (s *State) RemoveShelf(ctx context.Context, name string) ([]Shelf, error) {
	if _, ok := s.Shelf(name); !ok {
		return s.Shelves(), fmt.Errorf("货架 %q 不存在", strings.TrimSpace(name))
	}
	_, err := s.RemoveSource(ctx, name)
	return s.Shelves(), err
}

// SourceView 是当前来源的对外形态；Problem 非空表示配置不可用。
type SourceView struct {
	Source  string `json:"source"`
	Problem string `json:"problem,omitempty"`
}

// SourcesView 是来源注册表的对外形态；Problem 非空表示配置不可用。
type SourcesView struct {
	Sources []Source `json:"sources"`
	Problem string   `json:"problem,omitempty"`
}

// ShelvesView 是本机来源注册表的兼容对外形态。
type ShelvesView struct {
	Shelves []Shelf `json:"shelves"`
	Problem string  `json:"problem,omitempty"`
}

// CandidateSelector 按当前选择转发候选读取：
// 官方 → 官方读取器；本机 → 按目录现场构造读取器；网络 → 按清单地址现场构造读取器。
// 配置不可用或来源不可用时如实报错，绝不回退官方、不换其他来源。
type CandidateSelector struct {
	state    *State
	official releasecheck.CandidateReader
	remote   releasecheck.HTTPDoer
	kind     string
}

// NewCandidateSelector 构造某类别的候选选择器；只服务该类别的发布物。
func NewCandidateSelector(state *State, official releasecheck.CandidateReader, remote releasecheck.HTTPDoer, kind string) (*CandidateSelector, error) {
	if state == nil {
		return nil, fmt.Errorf("安装来源状态不能为空")
	}
	if official == nil {
		return nil, fmt.Errorf("官方候选读取器不能为空")
	}
	kind = strings.TrimSpace(kind)
	if kind != types.ReleaseArtifactKindTool && kind != types.ReleaseArtifactKindPlugin {
		return nil, fmt.Errorf("候选选择器不支持发布物类别 %q", kind)
	}
	return &CandidateSelector{state: state, official: official, remote: remote, kind: kind}, nil
}

// LatestCandidate 按当前来源读取候选。
func (s *CandidateSelector) LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*releasecheck.ReleaseCandidate, error) {
	if strings.TrimSpace(identity.Kind) != s.kind {
		return nil, fmt.Errorf("候选选择器类别 %q 不服务发布物类别 %q", s.kind, identity.Kind)
	}
	if problem := s.state.Problem(); problem != "" {
		return nil, fmt.Errorf("安装来源配置不可用：%s", problem)
	}
	sourceName := s.state.CurrentSource()
	if sourceName == OfficialSource {
		return s.official.LatestCandidate(ctx, identity)
	}
	source, ok := s.state.Source(sourceName)
	if !ok {
		return nil, fmt.Errorf("当前商店来源 %q 不是已注册来源", sourceName)
	}
	switch source.Kind {
	case SourceKindShelf:
		reader, err := releasecheck.NewLocalSourceReader(source.Name, source.Path, s.kind)
		if err != nil {
			return nil, fmt.Errorf("货架 %q 不可用：%w", source.Name, err)
		}
		return reader.LatestCandidate(ctx, identity)
	case SourceKindRemote:
		if s.remote == nil {
			return nil, fmt.Errorf("读取网络来源 %q 需要网络请求能力", source.Name)
		}
		reader, err := releasecheck.NewSourceReader(releasecheck.IndexSource{
			Name:     source.Name,
			Kind:     types.ReleaseSourceKindRemote,
			IndexURL: source.IndexURL,
		}, releasecheck.Config{Client: s.remote})
		if err != nil {
			return nil, fmt.Errorf("网络来源 %q 不可用：%w", source.Name, err)
		}
		return reader.LatestCandidate(ctx, identity)
	default:
		return nil, fmt.Errorf("来源 %q 的提供方式 %q 无效", source.Name, source.Kind)
	}
}
