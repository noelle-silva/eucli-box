package gateway

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

func (s *system) handleEventsWebSocket(w http.ResponseWriter, r *http.Request) {
	longTermKeyID := longTermKeyIDFromContext(r)
	if longTermKeyID == "" {
		if err := s.validateRequestKey(r); err != nil {
			writeError(w, err)
			return
		}
		if err := s.validateClientCompatibility(r); err != nil {
			writeError(w, err)
			return
		}
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	clearWebSocketDeadlines(conn)
	s.addConnection(conn)
	defer func() {
		s.removeConnection(conn)
		if longTermKeyID != "" && s.access != nil {
			s.access.UnregisterConnection(longTermKeyID, conn)
		}
		_ = conn.Close()
	}()
	if longTermKeyID != "" && s.access != nil {
		s.access.RegisterConnection(longTermKeyID, conn)
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	events, unsubscribe, err := s.runtime.Subscribe(ctx)
	if err != nil {
		_ = conn.WriteJSON(errorResponse{Error: errorPayloadForResponse(gatewayWebSocketFailed("failed to subscribe run events", err))})
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

func clearWebSocketDeadlines(conn *websocket.Conn) {
	_ = conn.SetReadDeadline(time.Time{})
	_ = conn.SetWriteDeadline(time.Time{})
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
