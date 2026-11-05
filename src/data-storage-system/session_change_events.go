package datastorage

import (
	"context"
	"sync"
	"time"

	"eucli-box/pkg/types"
)

// sessionChangeSubscriber 是单个订阅者的独立投递缓冲：
// 发布者只入缓冲，永不阻塞、永不丢弃；独立 goroutine 按入缓冲顺序投递。
type sessionChangeSubscriber struct {
	out    chan types.SessionChangeEvent
	done   chan struct{}
	signal chan struct{}

	mu      sync.Mutex
	pending []types.SessionChangeEvent
	closed  bool
}

func newSessionChangeSubscriber() *sessionChangeSubscriber {
	return &sessionChangeSubscriber{
		out:    make(chan types.SessionChangeEvent, 64),
		done:   make(chan struct{}),
		signal: make(chan struct{}, 1),
	}
}

func (sub *sessionChangeSubscriber) enqueue(event types.SessionChangeEvent) {
	sub.mu.Lock()
	if sub.closed {
		sub.mu.Unlock()
		return
	}
	sub.pending = append(sub.pending, event)
	sub.mu.Unlock()
	sub.notify()
}

func (sub *sessionChangeSubscriber) notify() {
	select {
	case sub.signal <- struct{}{}:
	default:
	}
}

func (sub *sessionChangeSubscriber) run() {
	defer close(sub.out)
	for {
		select {
		case <-sub.signal:
		case <-sub.done:
			return
		}
		for {
			sub.mu.Lock()
			if len(sub.pending) == 0 {
				sub.mu.Unlock()
				break
			}
			event := sub.pending[0]
			sub.pending[0] = types.SessionChangeEvent{}
			sub.pending = sub.pending[1:]
			sub.mu.Unlock()

			select {
			case sub.out <- event:
			case <-sub.done:
				return
			}
		}
	}
}

func (sub *sessionChangeSubscriber) close() {
	sub.mu.Lock()
	if sub.closed {
		sub.mu.Unlock()
		return
	}
	sub.closed = true
	sub.mu.Unlock()
	close(sub.done)
}

// SubscribeSessionChanges 订阅会话变更事件；每次成功写盘恰好投递一条。
func (s *system) SubscribeSessionChanges(ctx context.Context) (<-chan types.SessionChangeEvent, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, storageInvalid("subscription context is cancelled", err)
	}
	sub := newSessionChangeSubscriber()
	s.sessionChangesMu.Lock()
	if s.sessionChanges == nil {
		s.sessionChanges = map[*sessionChangeSubscriber]struct{}{}
	}
	s.sessionChanges[sub] = struct{}{}
	s.sessionChangesMu.Unlock()
	go sub.run()

	stop := make(chan struct{})
	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			close(stop)
			s.removeSessionChangeSubscriber(sub)
		})
	}
	go func() {
		select {
		case <-ctx.Done():
			unsubscribe()
		case <-stop:
		}
	}()
	return sub.out, unsubscribe, nil
}

func (s *system) removeSessionChangeSubscriber(sub *sessionChangeSubscriber) {
	s.sessionChangesMu.Lock()
	delete(s.sessionChanges, sub)
	s.sessionChangesMu.Unlock()
	sub.close()
}

func (s *system) publishSessionChange(event types.SessionChangeEvent) {
	s.sessionChangesMu.Lock()
	subscribers := make([]*sessionChangeSubscriber, 0, len(s.sessionChanges))
	for sub := range s.sessionChanges {
		subscribers = append(subscribers, sub)
	}
	s.sessionChangesMu.Unlock()
	for _, sub := range subscribers {
		sub.enqueue(event)
	}
}

// newSessionChangeEvent 在写盘前构造并校验本次变更事件：
// 事件不合法时宁可让写入在落盘前失败，也不写出残缺事实。
func newSessionChangeEvent(scope sessionScope, sessionID string, changeType types.SessionChangeType, version uint64, occurredAt time.Time) (types.SessionChangeEvent, error) {
	event := types.SessionChangeEvent{
		TargetKind: changeType.TargetKind(),
		Location:   scope.location(sessionID),
		ChangeType: changeType,
		Version:    version,
		OccurredAt: occurredAt.UTC(),
	}
	if err := event.Validate(); err != nil {
		return types.SessionChangeEvent{}, storageInvalid("session change event is invalid", err)
	}
	return event, nil
}
