package shellcommand

import (
	"strings"
	"sync"
	"unicode/utf8"
)

type capturedText struct {
	Text             string
	Truncated        bool
	InvalidUTF8      bool
	ReplacementCount int
}

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

func (b *limitedBuffer) Snapshot() capturedText {
	b.mu.Lock()
	defer b.mu.Unlock()
	return normalizeCapturedText(b.data, b.charLimit, b.truncated)
}

func normalizeCapturedText(data []byte, charLimit int, byteTruncated bool) capturedText {
	var builder strings.Builder
	builder.Grow(len(data))
	truncated := byteTruncated
	invalidCount := 0
	runeCount := 0
	for index := 0; index < len(data); {
		if charLimit > 0 && runeCount >= charLimit {
			truncated = true
			break
		}
		r, size := utf8.DecodeRune(data[index:])
		if r == utf8.RuneError && size == 1 {
			if byteTruncated && !utf8.FullRune(data[index:]) {
				truncated = true
				break
			}
			builder.WriteByte('?')
			invalidCount++
			index++
			runeCount++
			continue
		}
		builder.WriteRune(r)
		index += size
		runeCount++
	}
	return capturedText{Text: builder.String(), Truncated: truncated, InvalidUTF8: invalidCount > 0, ReplacementCount: invalidCount}
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
