package shellcommand

import "testing"

func TestLimitedBufferKeepsWholeOutputWithinBudget(t *testing.T) {
	buffer := newLimitedBuffer(5)
	if _, err := buffer.Write([]byte("01234")); err != nil {
		t.Fatalf("Write error = %v", err)
	}
	snapshot := buffer.Snapshot()
	if snapshot.Text != "01234" || snapshot.Truncated || snapshot.InvalidUTF8 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestLimitedBufferKeepsHeadAndTailAcrossChunks(t *testing.T) {
	buffer := newLimitedBuffer(5)
	for _, chunk := range []string{"0123", "4567", "89"} {
		if _, err := buffer.Write([]byte(chunk)); err != nil {
			t.Fatalf("Write(%q) error = %v", chunk, err)
		}
	}
	snapshot := buffer.Snapshot()
	// 10 bytes against charLimit 5: kept whole in head, then elided by runes.
	if snapshot.Text != "01…5 chars truncated…789" {
		t.Fatalf("unexpected snapshot text = %q", snapshot.Text)
	}
	if snapshot.OriginalBytes != 10 || snapshot.TotalLines != 0 {
		t.Fatalf("facts = %#v", snapshot)
	}
	if !snapshot.Truncated || snapshot.InvalidUTF8 || snapshot.ReplacementCount != 0 {
		t.Fatalf("flags = %#v", snapshot)
	}
}

func TestLimitedBufferKeepsHeadAndTailMultibyteOutput(t *testing.T) {
	buffer := newLimitedBuffer(2)
	if _, err := buffer.Write([]byte("abc界def")); err != nil {
		t.Fatalf("Write error = %v", err)
	}
	snapshot := buffer.Snapshot()
	// head "abc" + tail "def" joined by byte elision marker (cut inside 界).
	if snapshot.Text != "a…23 chars truncated…f" {
		t.Fatalf("snapshot = %q", snapshot.Text)
	}
	if !snapshot.Truncated {
		t.Fatalf("should be truncated: %#v", snapshot)
	}
}

func TestLimitedBufferDoesNotCountTruncatedMultibytePrefixAsInvalidUTF8(t *testing.T) {
	buffer := newLimitedBuffer(1)
	if _, err := buffer.Write([]byte("界界")); err != nil {
		t.Fatalf("Write error = %v", err)
	}
	snapshot := buffer.Snapshot()
	if snapshot.InvalidUTF8 || snapshot.ReplacementCount != 0 {
		t.Fatalf("should not count partial truncation as invalid: %#v", snapshot)
	}
	if !snapshot.Truncated {
		t.Fatalf("should be truncated: %#v", snapshot)
	}
}

func TestLimitedBufferKeepsInvalidUTF8Accounting(t *testing.T) {
	buffer := newLimitedBuffer(5)
	if _, err := buffer.Write([]byte{0xff, 'o', 'k'}); err != nil {
		t.Fatalf("Write error = %v", err)
	}
	snapshot := buffer.Snapshot()
	if snapshot.Text != "?ok" || snapshot.Truncated || !snapshot.InvalidUTF8 || snapshot.ReplacementCount != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestElideMiddleKeepsHeadTailAndMarker(t *testing.T) {
	merged := []rune("0123456789ABCDEFGHIJ")
	text := elideMiddle(merged, 10)
	if text != "01234…10 chars truncated…FGHIJ" {
		t.Fatalf("text = %q", text)
	}
}
