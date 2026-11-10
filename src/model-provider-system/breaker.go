package modelprovider

import (
	"sync"
	"time"
)

// memberCooldown 是成员失败后被跳过的时长：这段时间里任何策略都不会选它，
// 到期自动重新参与，不需要人工放行。
const memberCooldown = 30 * time.Second

// memberBreaker 记住"哪个成员刚失败过"，供所有选择策略统一跳过。
// 它只活在内存里：进程重启就忘掉，避免一次瞬时抖动变成长期误伤。
// 记录以成员为单位，跨模型组共享——同一家供应商的同一个模型在一处坏了，
// 另一处也没有理由再白试一次。
type memberBreaker struct {
	mu        sync.Mutex
	now       func() time.Time
	cooldown  time.Duration
	openUntil map[string]time.Time
}

func newMemberBreaker(cooldown time.Duration) *memberBreaker {
	if cooldown <= 0 {
		cooldown = memberCooldown
	}
	return &memberBreaker{now: time.Now, cooldown: cooldown, openUntil: map[string]time.Time{}}
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

// trip 记一次成员责任失败，把它关进小黑屋。
func (b *memberBreaker) trip(key string) {
	if key == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.openUntil[key] = b.now().Add(b.cooldown)
}

// clear 在成员成功响应后抹掉它的记录。
func (b *memberBreaker) clear(key string) {
	if key == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.openUntil, key)
}
