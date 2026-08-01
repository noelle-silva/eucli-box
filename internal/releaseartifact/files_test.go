package releaseartifact

import "testing"

func TestForbiddenPackagePathAllowsVerifiedAssetResources(t *testing.T) {
	roots := []string{"runtime/python"}
	if forbiddenPackagePath("runtime/python/Lib/site-packages/numpy/core/tests/data/astype_copy.pkl", roots) {
		t.Fatal("verified external asset resource was rejected")
	}
	if !forbiddenPackagePath("runtime/python/Lib/site-packages/numpy/core/tests/data/astype_copy.pkl", nil) {
		t.Fatal("unverified resource path was accepted")
	}
}

func TestForbiddenPackagePathKeepsSensitiveFilesForbiddenInsideAssets(t *testing.T) {
	roots := []string{"runtime/python"}
	for _, name := range []string{
		"runtime/python/.git/config",
		"runtime/python/.release/state.json",
		"runtime/python/.env.production",
		"runtime/python/private.key",
		"runtime/python/cache/state.bin",
		"runtime/python/settings/config.json",
		"runtime/python/secrets/token.txt",
		"runtime/python/credentials/user.json",
	} {
		if !forbiddenPackagePath(name, roots) {
			t.Fatalf("sensitive path was accepted: %s", name)
		}
	}
}
