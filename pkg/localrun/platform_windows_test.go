//go:build windows

package localrun

import (
	"testing"
	"time"
)

func TestProcessMatchesTreatsMissingProcessAsStale(t *testing.T) {
	matched, err := ProcessMatches(int(^uint32(0)), time.Now().UTC())
	if err != nil {
		t.Fatalf("ProcessMatches() error = %v", err)
	}
	if matched {
		t.Fatal("ProcessMatches() matched a missing process")
	}
}
