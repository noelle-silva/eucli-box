package shellcommand

import "testing"

func TestLimitedBufferKeepsRecentASCIIOutput(t *testing.T) {
	buffer := newLimitedBuffer(5)
	if _, err := buffer.Write([]byte("0123456789")); err != nil {
		t.Fatalf("Write error = %v", err)
	}
	snapshot := buffer.Snapshot()
	if snapshot.Text != "56789" || !snapshot.Truncated || snapshot.InvalidUTF8 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestLimitedBufferKeepsRecentOutputAcrossChunks(t *testing.T) {
	buffer := newLimitedBuffer(5)
	for _, chunk := range []string{"0123", "4567", "89"} {
		if _, err := buffer.Write([]byte(chunk)); err != nil {
			t.Fatalf("Write(%q) error = %v", chunk, err)
		}
	}
	snapshot := buffer.Snapshot()
	if snapshot.Text != "56789" || !snapshot.Truncated || snapshot.InvalidUTF8 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestLimitedBufferKeepsRecentMultibyteOutput(t *testing.T) {
	buffer := newLimitedBuffer(4)
	if _, err := buffer.Write([]byte("abc界def")); err != nil {
		t.Fatalf("Write error = %v", err)
	}
	snapshot := buffer.Snapshot()
	if snapshot.Text != "界def" || !snapshot.Truncated || snapshot.InvalidUTF8 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestLimitedBufferDoesNotCountTruncatedMultibytePrefixAsInvalidUTF8(t *testing.T) {
	buffer := newLimitedBuffer(1)
	if _, err := buffer.Write([]byte("界界")); err != nil {
		t.Fatalf("Write error = %v", err)
	}
	snapshot := buffer.Snapshot()
	if snapshot.Text != "界" || !snapshot.Truncated || snapshot.InvalidUTF8 || snapshot.ReplacementCount != 0 {
		t.Fatalf("snapshot = %#v", snapshot)
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
