package agentruntime

import (
	"context"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

func (s *system) Subscribe(ctx context.Context) (<-chan types.RunEvent, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, runtimeInvalid("subscription context is cancelled", err)
	}
	ch := make(chan types.RunEvent, 128)
	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()
	unsubscribe := func() {
		s.removeSubscriber(ch)
	}
	go func() {
		<-ctx.Done()
		s.removeSubscriber(ch)
	}()
	return ch, unsubscribe, nil
}

func (s *system) publish(runID string, eventType string, payload any) {
	event := types.RunEvent{ID: utils.NewID("event"), RunID: runID, Type: eventType, Payload: payload, CreatedAt: time.Now().UTC()}
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subscribers {
		select {
		case ch <- event:
		default:
			delete(s.subscribers, ch)
			close(ch)
		}
	}
}
