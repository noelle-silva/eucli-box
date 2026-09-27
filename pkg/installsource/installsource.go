package installsource

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

// OfficialSource 是官方发行来源的保留字；不允许作为货架名字。
const OfficialSource = "official"

// Shelf 是用户注册的一条货架记录：名字是对外身份（展示与选择都用它），路径是货架落点。
type Shelf struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Config 是安装来源配置的持久化形态：当前选择 + 货架注册表。
// Source 取值为 official 或某个货架名字；注册表按注册顺序保存。
type Config struct {
	Source  string  `json:"source"`
	Shelves []Shelf `json:"shelves"`
}

// DefaultConfig 是配置文件缺失时的初始形态：空注册表 + 官方源。
func DefaultConfig() Config {
	return Config{Source: OfficialSource, Shelves: []Shelf{}}
}

// Clone 深拷贝配置，避免调用方与状态共享底层切片。
func (c Config) Clone() Config {
	shelves := make([]Shelf, len(c.Shelves))
	copy(shelves, c.Shelves)
	return Config{Source: c.Source, Shelves: shelves}
}

// normalizeConfig 裁剪并校验配置，返回可保存的规范形态。
// 名字与路径裁剪首尾空白后非空；名字不得是保留字（不分大小写）且不得重复；
// 选择值必须是官方保留字或已注册货架。
func normalizeConfig(config Config) (Config, error) {
	source := strings.TrimSpace(config.Source)
	if source == "" {
		return Config{}, fmt.Errorf("安装来源配置缺少来源选择")
	}
	shelves := make([]Shelf, 0, len(config.Shelves))
	seen := make(map[string]struct{}, len(config.Shelves))
	for _, item := range config.Shelves {
		name := strings.TrimSpace(item.Name)
		path := strings.TrimSpace(item.Path)
		if name == "" {
			return Config{}, fmt.Errorf("货架名字不能为空")
		}
		if strings.EqualFold(name, OfficialSource) {
			return Config{}, fmt.Errorf("货架名字 %q 是保留字，不能使用", name)
		}
		if _, ok := seen[name]; ok {
			return Config{}, fmt.Errorf("货架名字 %q 重复", name)
		}
		if path == "" {
			return Config{}, fmt.Errorf("货架 %q 缺少路径", name)
		}
		seen[name] = struct{}{}
		shelves = append(shelves, Shelf{Name: name, Path: path})
	}
	if source != OfficialSource {
		if _, ok := seen[source]; !ok {
			return Config{}, fmt.Errorf("安装来源 %q 不是已注册货架", source)
		}
	}
	return Config{Source: source, Shelves: shelves}, nil
}

// Store 是安装来源配置的持久化接口，由 data-storage-system 实现。
type Store interface {
	LoadInstallSource(ctx context.Context) (Config, error)
	SaveInstallSource(ctx context.Context, config Config) error
}

// State 是货架注册表与当前选择的唯一事实源：内存值 + 持久化，一次操作一次原子保存。
// 初始配置非法时进入「配置不可用」状态：本体照常启动，来源读取如实报错；
// 重新设置官方源或注册一个货架即完成配置重建。
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
		return &State{config: Config{Shelves: []Shelf{}}, problem: fmt.Errorf("安装来源配置无效：%w", err), store: store}
	}
	return &State{config: normalized, store: store}
}

// NewStateUnavailable 构造「配置不可用」状态：加载失败的原因作为待修复原因保留。
func NewStateUnavailable(problem error, store Store) *State {
	if problem == nil {
		problem = fmt.Errorf("安装来源配置不可用")
	}
	return &State{config: Config{Shelves: []Shelf{}}, problem: problem, store: store}
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

// Shelves 返回按注册顺序排列的货架注册表副本；配置不可用时返回空表。
func (s *State) Shelves() []Shelf {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.problem != nil {
		return []Shelf{}
	}
	return s.config.Clone().Shelves
}

// Shelf 按名字读取货架记录；配置不可用时一律不存在。
func (s *State) Shelf(name string) (Shelf, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.problem != nil {
		return Shelf{}, false
	}
	for _, item := range s.config.Shelves {
		if item.Name == name {
			return item, true
		}
	}
	return Shelf{}, false
}

// ShelfPath 按名字读取货架路径；不存在时返回 false。
func (s *State) ShelfPath(name string) (string, bool) {
	shelf, ok := s.Shelf(name)
	return shelf.Path, ok
}

// commit 在锁内执行一次读改写：先校验，再持久化，成功后更新内存值并清除问题态。
// 失败时配置保持原样，并连同当前配置一起返回。
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

// SetSource 切换来源：必须是官方保留字或已注册货架，其余拒绝。
// 配置不可用时只有官方保留字可以完成重建。
func (s *State) SetSource(ctx context.Context, source string) (string, error) {
	next, err := s.commit(ctx, func(current Config, problem error) (Config, error) {
		value := strings.TrimSpace(source)
		if problem != nil {
			if value != OfficialSource {
				return current, fmt.Errorf("安装来源配置不可用（%v），请先重新设置官方源或注册货架", problem)
			}
			return Config{Source: OfficialSource, Shelves: []Shelf{}}, nil
		}
		current.Source = value
		return current, nil
	})
	return next.Source, err
}

// AddShelf 注册货架；重名、保留字、空值一律拒绝。
// 配置不可用时以新货架重建注册表，来源回到官方源。
func (s *State) AddShelf(ctx context.Context, name string, path string) ([]Shelf, error) {
	next, err := s.commit(ctx, func(current Config, problem error) (Config, error) {
		shelf := Shelf{Name: strings.TrimSpace(name), Path: strings.TrimSpace(path)}
		if problem != nil {
			return Config{Source: OfficialSource, Shelves: []Shelf{shelf}}, nil
		}
		current.Shelves = append(current.Shelves, shelf)
		return current, nil
	})
	return next.Shelves, err
}

// UpdateShelf 改名 / 改路径：至少提供一项；选中货架改名时选择跟随新名字。
// 配置不可用时无法修改货架。
func (s *State) UpdateShelf(ctx context.Context, name string, newName *string, newPath *string) ([]Shelf, error) {
	if newName == nil && newPath == nil {
		return s.Shelves(), fmt.Errorf("改货架必须提供新名字或新路径")
	}
	next, err := s.commit(ctx, func(current Config, problem error) (Config, error) {
		if problem != nil {
			return current, fmt.Errorf("安装来源配置不可用（%v），无法修改货架", problem)
		}
		target := strings.TrimSpace(name)
		for index := range current.Shelves {
			if current.Shelves[index].Name != target {
				continue
			}
			if newName != nil {
				current.Shelves[index].Name = strings.TrimSpace(*newName)
			}
			if newPath != nil {
				current.Shelves[index].Path = strings.TrimSpace(*newPath)
			}
			if current.Source == target {
				current.Source = current.Shelves[index].Name
			}
			return current, nil
		}
		return current, fmt.Errorf("货架 %q 不存在", target)
	})
	return next.Shelves, err
}

// RemoveShelf 删除货架；删除已选中的货架时选择回落官方源。
// 配置不可用时无法删除货架。
func (s *State) RemoveShelf(ctx context.Context, name string) ([]Shelf, error) {
	next, err := s.commit(ctx, func(current Config, problem error) (Config, error) {
		if problem != nil {
			return current, fmt.Errorf("安装来源配置不可用（%v），无法删除货架", problem)
		}
		target := strings.TrimSpace(name)
		kept := make([]Shelf, 0, len(current.Shelves))
		found := false
		for _, item := range current.Shelves {
			if item.Name == target {
				found = true
				continue
			}
			kept = append(kept, item)
		}
		if !found {
			return current, fmt.Errorf("货架 %q 不存在", target)
		}
		current.Shelves = kept
		if current.Source == target {
			current.Source = OfficialSource
		}
		return current, nil
	})
	return next.Shelves, err
}

// SourceView 是当前来源的对外形态；Problem 非空表示配置不可用。
type SourceView struct {
	Source  string `json:"source"`
	Problem string `json:"problem,omitempty"`
}

// ShelvesView 是货架注册表的对外形态；Problem 非空表示配置不可用。
type ShelvesView struct {
	Shelves []Shelf `json:"shelves"`
	Problem string  `json:"problem,omitempty"`
}

// CandidateSelector 按当前选择转发候选读取：
// 官方 → 官方读取器；货架 → 按货架路径现场构造本类别的货架读取器（每次读取都按最新注册表解析）。
// 配置不可用或货架不可用时如实报错，绝不回退官方、不换其他货架。
type CandidateSelector struct {
	state    *State
	official releasecheck.CandidateReader
	kind     string
}

// NewCandidateSelector 构造某类别的候选选择器；只服务该类别的发布物。
func NewCandidateSelector(state *State, official releasecheck.CandidateReader, kind string) (*CandidateSelector, error) {
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
	return &CandidateSelector{state: state, official: official, kind: kind}, nil
}

// LatestCandidate 按当前来源读取候选。
func (s *CandidateSelector) LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*releasecheck.ReleaseCandidate, error) {
	if strings.TrimSpace(identity.Kind) != s.kind {
		return nil, fmt.Errorf("候选选择器类别 %q 不服务发布物类别 %q", s.kind, identity.Kind)
	}
	if problem := s.state.Problem(); problem != "" {
		return nil, fmt.Errorf("安装来源配置不可用：%s", problem)
	}
	source := s.state.CurrentSource()
	if source == OfficialSource {
		return s.official.LatestCandidate(ctx, identity)
	}
	shelf, ok := s.state.Shelf(source)
	if !ok {
		return nil, fmt.Errorf("当前商店来源 %q 不是已注册货架", source)
	}
	reader, err := releasecheck.NewLocalSourceReader(shelf.Path, s.kind)
	if err != nil {
		return nil, fmt.Errorf("货架 %q 不可用：%w", shelf.Name, err)
	}
	return reader.LatestCandidate(ctx, identity)
}
