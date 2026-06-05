package modelprovider

import (
	cryptorand "crypto/rand"
	"math/big"

	"eucli-box/pkg/types"
)

func normalizeRotationStrategy(strategy types.RotationStrategy) types.RotationStrategy {
	if strategy == types.RotationStrategyWeightedRandom {
		return types.RotationStrategyWeightedRandom
	}
	return types.RotationStrategySequential
}

func positiveWeight(value int) int {
	if value < 1 {
		return 1
	}
	return value
}

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
