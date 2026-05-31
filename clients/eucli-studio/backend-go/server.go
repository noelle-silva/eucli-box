package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

type directServer struct {
	token   string
	service *service
	hub     *eventHub
}

func newDirectServer(token string, service *service, hub *eventHub) *directServer {
	return &directServer{token: token, service: service, hub: hub}
}

func (s *directServer) listenAndServe(ctx context.Context) error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	server := &http.Server{Handler: s}
	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()
	addr := listener.Addr().(*net.TCPAddr)
	writeReady(addr.Port)
	return server.Serve(listener)
}

func (s *directServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.URL.Query().Get("token")) != s.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	go s.handleConnection(r.Context(), conn)
}

func (s *directServer) handleConnection(ctx context.Context, conn *websocket.Conn) {
	defer conn.Close()
	s.hub.add(conn)
	defer s.hub.remove(conn)
	healthChecked := false
	for {
		var frame requestFrame
		if err := conn.ReadJSON(&frame); err != nil {
			return
		}
		if frame.ID == "" || frame.Type != "request" || frame.Method == "" {
			continue
		}
		if !healthChecked && frame.Method != "aiChat.healthCheck" {
			_ = conn.WriteJSON(errorResponseFor(frame.ID, newError("HEALTH_CHECK_REQUIRED", "请先完成 healthCheck")))
			continue
		}
		result, err := s.service.dispatch(ctx, frame.Method, frame.Params)
		if frame.Method == "aiChat.healthCheck" && err == nil {
			healthChecked = true
		}
		if err != nil {
			_ = conn.WriteJSON(errorResponseFor(frame.ID, err))
			continue
		}
		_ = conn.WriteJSON(okResponse(frame.ID, result))
	}
}

func writeReady(port int) {
	line, err := json.Marshal(readyFrame{Type: "ready", IPC: readyIPC{Mode: "direct", Transport: "local-websocket", URL: fmt.Sprintf("ws://127.0.0.1:%d", port), ProtocolVersion: directProtocolVersion}})
	if err != nil {
		log.Printf("ready frame marshal failed: %v", err)
		return
	}
	fmt.Println(string(line))
}
