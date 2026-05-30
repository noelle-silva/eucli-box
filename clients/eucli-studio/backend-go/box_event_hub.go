package main

import (
	"log"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type boxEventHub struct {
	wsURL       string
	baseURL     string
	token       string
	mu          sync.Mutex
	conn        *websocket.Conn
	subscribers map[string]map[int]func(boxRunEvent)
	nextSubID   int
	connected   bool
	closeCh     chan struct{}
	closeOnce   sync.Once

	toolConfCallback func(boxRunEvent)
	onDisconnect     func()

	maxBackoff  time.Duration
	maxFailures int

	loopAlive atomic.Bool
}

func newBoxEventHub(wsURL string, token string) *boxEventHub {
	h := &boxEventHub{
		wsURL:       wsURL,
		baseURL:     wsURL,
		token:       token,
		subscribers: make(map[string]map[int]func(boxRunEvent)),
		maxBackoff:  30 * time.Second,
		maxFailures: 10,
		closeCh:     make(chan struct{}),
	}
	go h.connectLoop()
	return h
}

func (h *boxEventHub) connectLoop() {
	h.loopAlive.Store(true)
	defer h.loopAlive.Store(false)

	backoff := 1 * time.Second
	failures := 0

	for {
		select {
		case <-h.closeCh:
			return
		default:
		}

		conn, err := h.dial()
		if err != nil {
			failures++
			log.Printf("boxEventHub: dial failed: %v (attempt %d, backoff %v)", err, failures, backoff)
			if failures >= h.maxFailures {
				log.Printf("boxEventHub: too many consecutive failures (%d), giving up", failures)
				h.mu.Lock()
				dc := h.onDisconnect
				h.mu.Unlock()
				if dc != nil {
					dc()
				}
				return
			}
			time.Sleep(backoff)
			backoff = nextBackoff(backoff, h.maxBackoff)
			continue
		}

		failures = 0
		backoff = 1 * time.Second

		h.mu.Lock()
		h.conn = conn
		h.connected = true
		h.mu.Unlock()

		log.Printf("boxEventHub: connected")
		h.readLoop(conn)

		h.mu.Lock()
		h.conn = nil
		h.connected = false
		dc := h.onDisconnect
		h.mu.Unlock()

		log.Printf("boxEventHub: disconnected, reconnecting...")
		if dc != nil {
			dc()
		}
	}
}

func (h *boxEventHub) dial() (*websocket.Conn, error) {
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	h.mu.Lock()
	fullURL := h.baseURL
	token := h.token
	h.mu.Unlock()
	if token != "" {
		fullURL += "?token=" + url.QueryEscape(token)
	}
	conn, _, err := dialer.Dial(fullURL, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (h *boxEventHub) updateConnection(wsURL string, token string) {
	h.mu.Lock()
	h.baseURL = wsURL
	h.token = token
	if h.conn != nil {
		h.conn.Close()
	}
	h.mu.Unlock()

	if !h.loopAlive.Load() {
		select {
		case <-h.closeCh:
			return
		default:
		}
		log.Printf("boxEventHub: connectLoop was dead, restarting")
		go h.connectLoop()
	}
}

func (h *boxEventHub) readLoop(conn *websocket.Conn) {
	defer conn.Close()

	lastPong := time.Now()
	conn.SetPongHandler(func(string) error {
		h.mu.Lock()
		lastPong = time.Now()
		h.mu.Unlock()
		return nil
	})

	done := make(chan struct{})
	defer close(done)

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				h.mu.Lock()
				alive := h.connected
				h.mu.Unlock()
				if !alive {
					return
				}
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
					log.Printf("boxEventHub: ping write failed: %v", err)
					return
				}
			case <-done:
				return
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				h.mu.Lock()
				stale := time.Since(lastPong) > 35*time.Second
				alive := h.connected
				h.mu.Unlock()
				if stale && alive {
					log.Printf("boxEventHub: pong timeout, forcing reconnect")
					conn.Close()
					return
				}
			case <-done:
				return
			}
		}
	}()

	for {
		var event boxRunEvent
		if err := conn.ReadJSON(&event); err != nil {
			log.Printf("boxEventHub: read error: %v", err)
			return
		}

		h.mu.Lock()
		cb := h.toolConfCallback
		h.mu.Unlock()

		if event.Type == "tool_confirmation_requested" && cb != nil {
			cb(event)
		}

		runID := strings.TrimSpace(event.RunID)
		if runID == "" {
			continue
		}

		h.mu.Lock()
		subs := h.subscribers[runID]
		listeners := make([]func(boxRunEvent), 0, len(subs))
		for _, fn := range subs {
			listeners = append(listeners, fn)
		}
		h.mu.Unlock()

		for _, fn := range listeners {
			fn(event)
		}
	}
}

func (h *boxEventHub) Close() {
	h.closeOnce.Do(func() {
		close(h.closeCh)
		h.mu.Lock()
		if h.conn != nil {
			h.conn.Close()
		}
		h.mu.Unlock()
	})
}

func (h *boxEventHub) subscribe(runID string, onEvent func(boxRunEvent)) func() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.subscribers[runID] == nil {
		h.subscribers[runID] = make(map[int]func(boxRunEvent))
	}
	id := h.nextSubID
	h.nextSubID++
	h.subscribers[runID][id] = onEvent

	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if subs := h.subscribers[runID]; subs != nil {
			delete(subs, id)
			if len(subs) == 0 {
				delete(h.subscribers, runID)
			}
		}
	}
}

func (h *boxEventHub) setToolConfirmationCallback(cb func(boxRunEvent)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.toolConfCallback = cb
}

func (h *boxEventHub) setOnDisconnect(cb func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onDisconnect = cb
}

func (h *boxEventHub) isConnected() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.connected
}

func nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}
