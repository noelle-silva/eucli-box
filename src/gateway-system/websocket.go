package gateway

import (
	"context"
	"net/http"

	"github.com/gorilla/websocket"
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
		_ = conn.WriteJSON(map[string]any{"error": gatewayWebSocketFailed("failed to subscribe runtime events", err).Error()})
		return
	}
	defer unsubscribe()
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
