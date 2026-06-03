package shellcommand

import (
	"strings"
	"sync"
)

type limitedBuffer struct {
	mu        sync.Mutex
	charLimit int
	byteLimit int
	data      []byte
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	byteLimit := limit * 4
	if byteLimit < limit {
		byteLimit = limit
	}
	return &limitedBuffer{charLimit: limit, byteLimit: byteLimit}
}

func (b *limitedBuffer) Write(payload []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.byteLimit > 0 && len(b.data) < b.byteLimit {
		remaining := b.byteLimit - len(b.data)
		if len(payload) <= remaining {
			b.data = append(b.data, payload...)
		} else {
			b.data = append(b.data, payload[:remaining]...)
			b.truncated = true
		}
	} else if len(payload) > 0 {
		b.truncated = true
	}
	return len(payload), nil
}

func (b *limitedBuffer) Snapshot() (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	text := strings.ToValidUTF8(string(b.data), "?")
	truncated := b.truncated
	if b.charLimit > 0 {
		runes := []rune(text)
		if len(runes) > b.charLimit {
			text = string(runes[:b.charLimit])
			truncated = true
		}
	}
	return text, truncated
}

type streamCapture struct {
	stream   *limitedBuffer
	combined *limitedBuffer
}

func (w streamCapture) Write(payload []byte) (int, error) {
	if _, err := w.stream.Write(payload); err != nil {
		return 0, err
	}
	return w.combined.Write(payload)
}
