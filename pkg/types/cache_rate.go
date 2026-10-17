package types

// CacheRateFromHitMiss 计算缓存率：命中量 ÷（命中量＋未命中量）。
// 每个输入量非命中即未命中；任一量为负或总量为 0 时缓存率无意义，返回 nil 表示未知。
func CacheRateFromHitMiss(hit int, miss int) *float64 {
	if hit < 0 || miss < 0 {
		return nil
	}
	total := hit + miss
	if total <= 0 {
		return nil
	}
	rate := float64(hit) / float64(total)
	return &rate
}
