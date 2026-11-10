package modelprovider

import (
	"context"

	"eucli-box/pkg/types"
)

// planCandidate 是一个候选：解析好的供应商与模型。
type planCandidate struct {
	provider types.Provider
	model    types.ModelInfo
	// key 是这个候选在熔断表里的身份；为空表示不参与熔断
	// （单供应商坐标没有替补成员，跳过它没有意义）。
	key string
}

// label 是候选在错误汇总里显示的名字。
func (c planCandidate) label() string {
	if c.key != "" {
		return c.key
	}
	return memberKey(c.provider.ID, c.model.ID)
}

// memberKey 是成员在熔断表里的统一身份：同一家供应商的同一个注册模型，
// 无论出现在哪个组里都共用一条记录。
func memberKey(providerID string, modelID string) string {
	if providerID == "" || modelID == "" {
		return ""
	}
	return providerID + "/" + modelID
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
	return isMemberFailure(err)
}

// planRequest 把坐标翻译成执行计划：先剔除小黑屋里的成员，再交给策略选人。
// fallback 组把剩余成员按顺序展开成兜底链，其余策略仍只选出一个成员。
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
	indexes := s.availableMemberIndexes(model)
	if model.Strategy != types.RotationStrategyFallback {
		candidate, err := s.selectGroupMember(ctx, group, model, indexes)
		if err != nil {
			return requestPlan{}, err
		}
		return requestPlan{candidates: []planCandidate{candidate}}, nil
	}
	return s.buildFallbackChain(ctx, model, indexes)
}

// availableMemberIndexes 返回本次可以参与选择的成员下标：刚失败过的成员被跳过。
// 全都还在小黑屋里时返回全部成员——宁可去试一个坏成员，也不能让整组直接不可用。
func (s *system) availableMemberIndexes(model types.ModelGroupModel) []int {
	all := make([]int, 0, len(model.Members))
	available := make([]int, 0, len(model.Members))
	for index, member := range model.Members {
		all = append(all, index)
		if s.breaker.isCooling(memberKey(member.ProviderID, member.ModelID)) {
			continue
		}
		available = append(available, index)
	}
	if len(available) == 0 {
		return all
	}
	return available
}
