package shellcommand

import (
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
	if len(payload) == 0 {
		return len(payload), nil
	}
	if b.byteLimit <= 0 {
		b.truncated = true
		return len(payload), nil
	}
	b.data = append(b.data, payload...)
	if len(b.data) > b.byteLimit {
		drop := len(b.data) - b.byteLimit
		b.data = b.data[drop:]
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
	truncated := byteTruncated
	if byteTruncated {
		data = trimLeadingPartialUTF8(data)
	}
	runes := make([]rune, 0, len(data))
	invalidCount := 0
	for index := 0; index < len(data); {
		r, size := utf8.DecodeRune(data[index:])
		if r == utf8.RuneError && size == 1 {
			if byteTruncated && !utf8.FullRune(data[index:]) {
				truncated = true
				break
			}
			runes = append(runes, '?')
			invalidCount++
			index++
			continue
		}
		runes = append(runes, r)
		index += size
	}
	if charLimit > 0 && len(runes) > charLimit {
		runes = runes[len(runes)-charLimit:]
		truncated = true
	}
	return capturedText{Text: string(runes), Truncated: truncated, InvalidUTF8: invalidCount > 0, ReplacementCount: invalidCount}
}

func trimLeadingPartialUTF8(data []byte) []byte {
	index := 0
	for index < len(data) && data[index]&0xC0 == 0x80 {
		index++
	}
	return data[index:]
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
