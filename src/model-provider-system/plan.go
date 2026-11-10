package modelprovider

import (
	"context"

	"eucli-box/pkg/types"
)

// planCandidate 是一个候选：解析好的供应商与模型。
type planCandidate struct {
	provider types.Provider
	model    types.ModelInfo
}

// label 是候选的可读标识，用于兜底失败时的错误汇总。
func (c planCandidate) label() string {
	return c.provider.ID + "/" + c.model.ID
}

// requestPlan 是一次模型请求的执行计划：一个候选就是普通请求，
// 多个候选表示 fallback 组按成员顺序顺位兜底。
type requestPlan struct {
	candidates []planCandidate
}

// fallback 表示这是一条兜底链。
func (p requestPlan) fallback() bool {
	return len(p.candidates) > 1
}

// isLast 判断是否已经试到链尾。
func (p requestPlan) isLast(index int) bool {
	return index >= len(p.candidates)-1
}

// shouldTryNext 判断第 index 个候选失败后是否还有继续往下试的理由。
// 已经分发过内容就不能再换人：用户看到的内容收不回来，换人只会变成重放。
func (p requestPlan) shouldTryNext(index int, err error, dispatched bool) bool {
	if dispatched || p.isLast(index) {
		return false
	}
	return shouldSwitchMember(err)
}

// planRequest 把坐标翻译成执行计划：fallback 组成员全部展开成候选链，
// 其余策略仍只选出一个成员。展开候选链不推进轮询游标。
func (s *system) planRequest(ctx context.Context, coordinate types.ModelCoordinate) (requestPlan, error) {
	if !isModelGroupCoordinate(coordinate) {
		provider, model, err := s.resolveProviderCoordinate(ctx, coordinate)
		if err != nil {
			return requestPlan{}, err
		}
		return requestPlan{candidates: []planCandidate{{provider: provider, model: model}}}, nil
	}
	group, model, err := s.loadModelGroupModel(ctx, coordinate)
	if err != nil {
		return requestPlan{}, err
	}
	if model.Strategy != types.RotationStrategyFallback {
		candidate, err := s.selectGroupMember(ctx, group, model)
		if err != nil {
			return requestPlan{}, err
		}
		return requestPlan{candidates: []planCandidate{candidate}}, nil
	}
	return s.buildFallbackChain(ctx, model)
}
