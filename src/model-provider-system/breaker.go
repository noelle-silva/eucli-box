package modelprovider

import (
	"sync"
	"time"
)

// memberBreaker 记住"哪个成员刚失败过"，供所有选择策略统一跳过。
// 它只活在内存里：进程重启就忘掉，避免一次瞬时抖动变成长期误伤。
// 记录以成员为单位，跨模型组共享——同一家供应商的同一个模型在一处坏了，
// 另一处也没有理由再白试一次。
type memberBreaker struct {
	mu        sync.Mutex
	now       func() time.Time
	openUntil map[string]time.Time
	strikes   map[string]memberStrikes
}

// memberStrikes 是一个成员当前攒到的连续失败次数，以及最后一次失败的时刻：
// lastAt 用来判断两次失败是不是"连着"，超出窗口的旧计数会被下一次失败冲掉。
// 表里最多只有成员数量那么多条记录（每次失败覆盖同一个 key）。
type memberStrikes struct {
	count  int
	lastAt time.Time
}

func newMemberBreaker() *memberBreaker {
	return &memberBreaker{
		now:       time.Now,
		openUntil: map[string]time.Time{},
		strikes:   map[string]memberStrikes{},
	}
}

// isCooling 判断成员是否还在小黑屋。
// key 为空表示这个候选不参与熔断（例如单供应商坐标，跳过它没有人可以替补）。
func (b *memberBreaker) isCooling(key string) bool {
	if key == "" {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	until, ok := b.openUntil[key]
	if !ok {
		return false
	}
	if b.now().Before(until) {
		return true
	}
	// 冷却已到期：顺手清掉过期记录，表不会随成员数量以外的东西增长。
	delete(b.openUntil, key)
	return false
}

// recordFailure 记一次成员责任失败，按档位决定是立刻关人还是攒够次数再关：
// 硬伤与限流一次就关，偶发档连两次才关。
func (b *memberBreaker) recordFailure(key string, grade memberFailureGrade) {
	if key == "" || grade.tier == breakerTierNone {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	switch grade.tier {
	case breakerTierHard:
		b.tripLocked(key, now, breakerHardCooldown)
	case breakerTierRateLimited:
		b.tripLocked(key, now, rateLimitCooldown(grade.retryAfter))
	case breakerTierTransient:
		if b.countStrikeLocked(key, now) >= breakerStrikeThreshold {
			b.tripLocked(key, now, breakerCooldown)
		}
	}
}

// tripLocked 把成员关进小黑屋，并清掉它的连续失败计数：
// 关人这件事已经让计数完成使命，放出来之后的第一下失败要重新数，不能一碰就又关。
// 已经在冷却中的成员只延长不缩短：它在硬伤档被关了 5 分钟，
// 不该因为后来一次偶发失败就降格成 30 秒。
func (b *memberBreaker) tripLocked(key string, now time.Time, cooldown time.Duration) {
	until := now.Add(cooldown)
	if current, ok := b.openUntil[key]; ok && current.After(until) {
		until = current
	}
	b.openUntil[key] = until
	delete(b.strikes, key)
}

// countStrikeLocked 记一次偶发档失败，并给出它是"连着"的第几次；
// 上一次失败已经落在窗口之外时，这次重新从第一次算起。
func (b *memberBreaker) countStrikeLocked(key string, now time.Time) int {
	strikes := b.strikes[key]
	if now.Sub(strikes.lastAt) > breakerStrikeWindow {
		strikes.count = 0
	}
	strikes.count++
	strikes.lastAt = now
	b.strikes[key] = strikes
	return strikes.count
}

// clear 在成员成功响应后抹掉它的记录：冷却与连续失败计数一起清零。
func (b *memberBreaker) clear(key string) {
	if key == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.openUntil, key)
	delete(b.strikes, key)
}

// rateLimitCooldown 取限流档的关停时长：上游给了 Retry-After 就听它的，
// 没给（或给了一个等于没说的 0）就退回默认时长。
func rateLimitCooldown(retryAfter time.Duration) time.Duration {
	if retryAfter <= 0 {
		return breakerCooldown
	}
	return retryAfter
}
