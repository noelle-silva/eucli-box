package gateway

import (
	"context"
	"errors"
	"net/http"

	"github.com/gorilla/websocket"

	apperrors "eucli-box/pkg/errors"
)

func (s *system) handleEventsWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.addConnection(conn)
	defer func() {
		s.removeConnection(conn)
		_ = conn.Close()
	}()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	events, unsubscribe, err := s.runtime.Subscribe(ctx)
	if err != nil {
		_ = conn.WriteJSON(errorResponse{Error: responseError{Code: "gateway.websocket_failed", Message: err.Error(), System: systemName}})
		return
	}
	defer unsubscribe()
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	}()
	for event := range events {
		if err := conn.WriteJSON(event); err != nil {
			return
		}
	}
}

func (s *system) addConnection(conn *websocket.Conn) {
	s.wsMu.Lock()
	s.connections[conn] = struct{}{}
	s.wsMu.Unlock()
}

func (s *system) removeConnection(conn *websocket.Conn) {
	s.wsMu.Lock()
	delete(s.connections, conn)
	s.wsMu.Unlock()
}

func (s *system) closeConnections() {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	for conn := range s.connections {
		_ = conn.Close()
		delete(s.connections, conn)
	}
}

func resolveInnerAppError(err error) *apperrors.AppError {
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		if cause := appErr.Unwrap(); cause != nil {
			var inner *apperrors.AppError
			if errors.As(cause, &inner) {
				return inner
			}
		}
		return appErr
	}
	return nil
}
