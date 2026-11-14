package modelprovider

import (
	cryptorand "crypto/rand"
	"math/big"

	"eucli-box/pkg/types"
)

func normalizeRotationStrategy(strategy types.RotationStrategy) types.RotationStrategy {
	switch strategy {
	case types.RotationStrategyWeightedRandom:
		return types.RotationStrategyWeightedRandom
	case types.RotationStrategyFallback:
		return types.RotationStrategyFallback
	default:
		return types.RotationStrategySequential
	}
}

// rotationStrategyOptions 是模型组策略清单的唯一事实源：
// value 是服务端唯一认的提交值，label 与 description 只用于客户端渲染下拉项。
// 顺序即展示顺序，调用方不要重排。
var rotationStrategyOptions = []types.RotationStrategyOption{
	{Value: types.RotationStrategySequential, Label: "顺序轮询", Description: "按成员顺序轮流使用，均摊负载"},
	{Value: types.RotationStrategyWeightedRandom, Label: "权重随机", Description: "按成员权重随机抽取"},
	{Value: types.RotationStrategyFallback, Label: "故障转移", Description: "始终从第一位开始，失败才顺位换下一个"},
}

// RotationStrategies 返回模型组可选的策略清单。
// 返回副本：清单是全局共用的事实，调用方改它不该影响别人拿到的下一份。
func (s *system) RotationStrategies() []types.RotationStrategyOption {
	options := make([]types.RotationStrategyOption, len(rotationStrategyOptions))
	copy(options, rotationStrategyOptions)
	return options
}

func positiveWeight(value int) int {
	if value < 1 {
		return 1
	}
	return value
}

// pickRotatedIndex 按轮询类策略选位置：fallback 不走这里，它的顺序由兜底链自己决定。
func (s *system) pickRotatedIndex(cursorKey string, strategy types.RotationStrategy, weights []int) (int, error) {
	if len(weights) == 0 {
		return -1, providerInvalid("rotation candidate is required", nil)
	}
	if normalizeRotationStrategy(strategy) == types.RotationStrategyWeightedRandom {
		return weightedRandomIndex(weights)
	}

	s.rotationMu.Lock()
	defer s.rotationMu.Unlock()
	if s.modelCursors == nil {
		s.modelCursors = map[string]int{}
	}
	index := s.modelCursors[cursorKey] % len(weights)
	if index < 0 {
		index = 0
	}
	s.modelCursors[cursorKey] = index + 1
	return index, nil
}

// pickProviderKeyIndex 在供应商的多把 API Key 之间选一把。
// fallback 只描述模型间的兜底关系，对同供应商的 Key 没有意义，这里会退化成顺序轮换。
func (s *system) pickProviderKeyIndex(providerID string, strategy types.RotationStrategy, weights []int) (int, error) {
	if len(weights) == 0 {
		return -1, providerInvalid("provider has no enabled api key", nil)
	}
	if normalizeRotationStrategy(strategy) == types.RotationStrategyWeightedRandom {
		return weightedRandomIndex(weights)
	}

	s.rotationMu.Lock()
	defer s.rotationMu.Unlock()
	if s.keyCursors == nil {
		s.keyCursors = map[string]int{}
	}
	index := s.keyCursors[providerID] % len(weights)
	if index < 0 {
		index = 0
	}
	s.keyCursors[providerID] = index + 1
	return index, nil
}

func weightedRandomIndex(weights []int) (int, error) {
	total := 0
	for _, weight := range weights {
		total += positiveWeight(weight)
	}
	if total <= 0 {
		return -1, providerInvalid("rotation weight is invalid", nil)
	}
	n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(total)))
	if err != nil {
		return -1, providerInvalid("failed to choose weighted rotation candidate", err)
	}
	pick := int(n.Int64())
	for index, weight := range weights {
		pick -= positiveWeight(weight)
		if pick < 0 {
			return index, nil
		}
	}
	return len(weights) - 1, nil
}
