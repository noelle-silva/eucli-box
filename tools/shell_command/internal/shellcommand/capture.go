package shellcommand

import (
	"bytes"
	"fmt"
	"sync"
	"unicode/utf8"
)

// maxOutputBytes is the design hard cap for displayed command output (1MB).
const maxOutputBytes = 1 << 20

type capturedText struct {
	Text             string
	Truncated        bool
	InvalidUTF8      bool
	ReplacementCount int
	OriginalBytes    int64
	TotalLines       int64
}

// limitedBuffer keeps a head window and a tail window of the written stream.
// The middle of oversized output is dropped while writing; the snapshot joins
// the two windows with a single elision marker when needed.
type limitedBuffer struct {
	mu         sync.Mutex
	charLimit  int
	headLimit  int
	tailLimit  int
	head       []byte
	tail       []byte
	totalBytes int64
	fullLines  int64
	truncated  bool
}

func newLimitedBuffer(charLimit int) *limitedBuffer {
	byteBudget := charLimit * 4
	if byteBudget < charLimit {
		byteBudget = charLimit
	}
	if byteBudget > maxOutputBytes {
		byteBudget = maxOutputBytes
	}
	headLimit := byteBudget / 2
	tailLimit := byteBudget - headLimit
	return &limitedBuffer{charLimit: charLimit, headLimit: headLimit, tailLimit: tailLimit}
}

func (b *limitedBuffer) Write(payload []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(payload) == 0 {
		return 0, nil
	}
	originalLen := len(payload)
	b.totalBytes += int64(originalLen)
	b.fullLines += int64(bytes.Count(payload, []byte{'\n'}))
	if len(b.head) < b.headLimit {
		room := b.headLimit - len(b.head)
		if room >= originalLen {
			b.head = append(b.head, payload...)
			return originalLen, nil
		}
		b.head = append(b.head, payload[:room]...)
		payload = payload[room:]
	}
	b.tail = append(b.tail, payload...)
	if len(b.tail) > b.tailLimit {
		b.truncated = true
		b.tail = b.tail[len(b.tail)-b.tailLimit:]
	}
	return originalLen, nil
}

func (b *limitedBuffer) Snapshot() capturedText {
	b.mu.Lock()
	defer b.mu.Unlock()
	head := trimTrailingPartialUTF8(b.head)
	tail := trimLeadingPartialUTF8(b.tail)
	omitted := b.totalBytes - int64(len(head)) - int64(len(tail))
	raw := make([]byte, 0, len(head)+len(tail)+40)
	raw = append(raw, head...)
	if b.truncated {
		raw = append(raw, fmt.Sprintf("…%d bytes truncated…", omitted)...)
	}
	raw = append(raw, tail...)
	runes := decodeRunes(raw)
	text := string(runes)
	truncated := b.truncated
	if len(runes) > b.charLimit {
		text = elideMiddle(runes, b.charLimit)
		truncated = true
	}
	return capturedText{
		Text:             text,
		Truncated:        truncated,
		InvalidUTF8:      invalidRunes(raw) > 0,
		ReplacementCount: invalidRunes(raw),
		OriginalBytes:    b.totalBytes,
		TotalLines:       b.fullLines,
	}
}

// elideMiddle cuts merged runes down to charLimit, keeping the first and last
// segments joined by a single middle marker. charLimit of 1 keeps only the
// final rune.
func elideMiddle(merged []rune, charLimit int) string {
	if len(merged) <= charLimit {
		return string(merged)
	}
	omitted := len(merged) - charLimit
	marker := fmt.Sprintf("…%d chars truncated…", omitted)
	headWidth := charLimit / 2
	tailWidth := charLimit - headWidth
	return string(merged[:headWidth]) + marker + string(merged[len(merged)-tailWidth:])
}

// decodeRunes converts raw bytes to runes, replacing invalid UTF-8 with '?'.
func decodeRunes(data []byte) []rune {
	runes := make([]rune, 0, len(data))
	for index := 0; index < len(data); {
		r, size := utf8.DecodeRune(data[index:])
		if r == utf8.RuneError && size == 1 {
			runes = append(runes, '?')
			index++
			continue
		}
		runes = append(runes, r)
		index += size
	}
	return runes
}

func invalidRunes(data []byte) int {
	count := 0
	for index := 0; index < len(data); {
		r, size := utf8.DecodeRune(data[index:])
		if r == utf8.RuneError && size == 1 {
			count++
			index++
			continue
		}
		index += size
	}
	return count
}

// trimLeadingPartialUTF8 drops continuation bytes at the start of a window
// that belong to a rune cut off before the window.
func trimLeadingPartialUTF8(data []byte) []byte {
	index := 0
	for index < len(data) && data[index]&0xC0 == 0x80 {
		index++
	}
	return data[index:]
}

// trimTrailingPartialUTF8 drops an incomplete rune at the end of a window.
func trimTrailingPartialUTF8(data []byte) []byte {
	index := 0
	for index < len(data) {
		if !utf8.FullRune(data[index:]) {
			return data[:index]
		}
		_, size := utf8.DecodeRune(data[index:])
		index += size
	}
	return data
}

type streamCapture struct {
	stream   *limitedBuffer
	combined *limitedBuffer
	onChunk  func(payload []byte)
}

func (w streamCapture) Write(payload []byte) (int, error) {
	if w.onChunk != nil && len(payload) > 0 {
		w.onChunk(payload)
	}
	if _, err := w.stream.Write(payload); err != nil {
		return 0, err
	}
	return w.combined.Write(payload)
}
