package boxrelease

import (
	"testing"

	"eucli-box/pkg/release"
)

func TestLoadReturnsValidBoxRelease(t *testing.T) {
	info, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := release.ValidateVersion(info.Version); err != nil {
		t.Fatalf("version %q is invalid: %v", info.Version, err)
	}
	if err := release.ValidateVersion(info.DataVersion); err != nil {
		t.Fatalf("data version %q is invalid: %v", info.DataVersion, err)
	}
}
