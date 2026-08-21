package installsource

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

// Kind 是发布物安装来源类别。
// official 读取固定官方发行；development 读取当前源码制作的本地成品。
// 开发来源只能由开发模式显式开启，不能根据文件存在或构建方式猜测。
type Kind string

const (
	KindOfficial    Kind = "official"
	KindDevelopment Kind = "development"
)

// ParseKind 解析来源类别；空值或未知值返回错误，不静默归一。
func ParseKind(value string) (Kind, error) {
	kind := Kind(strings.TrimSpace(value))
	if !kind.Valid() {
		return "", fmt.Errorf("不支持的安装来源 %q", value)
	}
	return kind, nil
}

func (k Kind) Valid() bool {
	return k == KindOfficial || k == KindDevelopment
}

func (k Kind) String() string { return string(k) }

// SourceStore 是安装来源状态的持久化接口，由 data-storage-system 实现。
type SourceStore interface {
	LoadInstallSource(ctx context.Context) (Kind, error)
	SaveInstallSource(ctx context.Context, kind Kind) error
}

// State 是安装来源的当前状态：内存值 + 持久化 + 可修改性。
// mutable 表示来源是否可以切换，由开发模式标记决定；正式模式不可切。
type State struct {
	mu      sync.RWMutex
	current Kind
	mutable bool
	store   SourceStore
}

// NewState 构造来源状态；initial 必须是合法值。
func NewState(initial Kind, mutable bool, store SourceStore) (*State, error) {
	if !initial.Valid() {
		return nil, fmt.Errorf("无效的安装来源初始值 %q", initial)
	}
	return &State{current: initial, mutable: mutable, store: store}, nil
}

// Current 返回当前安装来源状态。
func (s *State) Current() Kind {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// Set 切换安装来源：先校验可修改性与合法值，再持久化，成功后更新内存值。
func (s *State) Set(ctx context.Context, kind Kind) (Kind, error) {
	if !kind.Valid() {
		return s.Current(), fmt.Errorf("不支持的安装来源 %q", kind)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.mutable {
		return s.current, fmt.Errorf("正式模式不允许切换安装来源")
	}
	if s.store != nil {
		if err := s.store.SaveInstallSource(ctx, kind); err != nil {
			return s.current, err
		}
	}
	s.current = kind
	return kind, nil
}

// StateView 是对外暴露的来源状态形态。
type StateView struct {
	Kind Kind `json:"kind"`
}

// View 返回当前状态的对外形态。
func (s *State) View() StateView {
	return StateView{Kind: s.Current()}
}

// CandidateSelector 按当前安装来源状态转发候选读取。
// official → 官方读取器；development → 开发读取器（未激活时快速失败，不回退官方）。
type CandidateSelector struct {
	current     func() Kind
	official    releasecheck.CandidateReader
	development releasecheck.CandidateReader
}

// NewCandidateSelector 构造候选选择器。
func NewCandidateSelector(current func() Kind, official releasecheck.CandidateReader, development releasecheck.CandidateReader) (*CandidateSelector, error) {
	if current == nil {
		return nil, fmt.Errorf("安装来源读取函数不能为空")
	}
	if official == nil {
		return nil, fmt.Errorf("官方候选读取器不能为空")
	}
	return &CandidateSelector{current: current, official: official, development: development}, nil
}

// LatestCandidate 按当前状态读取候选；开发状态下开发读取器未激活时如实报错。
func (s *CandidateSelector) LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*releasecheck.ReleaseCandidate, error) {
	switch s.current() {
	case KindDevelopment:
		if s.development == nil {
			return nil, fmt.Errorf("开发来源未激活或开发成品资料不完整")
		}
		return s.development.LatestCandidate(ctx, identity)
	default:
		return s.official.LatestCandidate(ctx, identity)
	}
}
