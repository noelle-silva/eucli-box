//go:build !windows

package datastorage

import (
	"errors"
	"testing"
)

func TestIsRetryableCommitErrorIsPlatformNeutral(t *testing.T) {
	if isRetryableCommitError(errors.New("any commit failure")) {
		t.Fatal("non-windows commit errors must not be retried")
	}
}
