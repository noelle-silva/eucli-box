package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type eventHub struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]struct{}
}

func newEventHub() *eventHub {
	return &eventHub{clients: map[*websocket.Conn]struct{}{}}
}

func (h *eventHub) add(conn *websocket.Conn) {
	h.mu.Lock()
	h.clients[conn] = struct{}{}
	h.mu.Unlock()
}

func (h *eventHub) remove(conn *websocket.Conn) {
	h.mu.Lock()
	delete(h.clients, conn)
	h.mu.Unlock()
}

func (h *eventHub) broadcast(event eventFrame) {
	h.mu.Lock()
	clients := make([]*websocket.Conn, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.Unlock()
	for _, client := range clients {
		_ = client.WriteJSON(event)
	}
}

type eventBridge struct {
	config *configStore
	hub    *eventHub
}

func newEventBridge(config *configStore, hub *eventHub) *eventBridge {
	return &eventBridge{config: config, hub: hub}
}

func (b *eventBridge) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := b.connectOnce(ctx); err != nil {
			log.Printf("event bridge: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(1500 * time.Millisecond):
		}
	}
}

func (b *eventBridge) connectOnce(ctx context.Context) error {
	cfg, err := b.config.requireConfigured()
	if err != nil {
		return err
	}
	target, err := url.Parse(strings.TrimRight(cfg.EucliBoxURL, "/") + "/ws/events")
	if err != nil {
		return err
	}
	if target.Scheme == "http" {
		target.Scheme = "ws"
	} else if target.Scheme == "https" {
		target.Scheme = "wss"
	}
	header := http.Header{}
	if cfg.EucliBoxKey != "" {
		header.Set("Authorization", "Bearer "+cfg.EucliBoxKey)
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, target.String(), header)
	if err != nil {
		return err
	}
	defer conn.Close()
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var decoded any
		if err := json.Unmarshal(payload, &decoded); err != nil {
			continue
		}
		b.hub.broadcast(eventFrame{Type: "event", Name: "eucliBox.run.event", Payload: decoded})
	}
}
