package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"devtools/common/releasecredentials"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
)

func repositoryRoot(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "."
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("仓库根目录无效")
	}
	if _, err := os.Stat(filepath.Join(absolute, "go.mod")); err != nil {
		return "", fmt.Errorf("仓库根目录缺少 go.mod")
	}
	return filepath.Clean(absolute), nil
}

func resolveTarget(value string) (releasecatalog.Catalog, types.ReleaseArtifactIdentity, error) {
	catalog, err := releasecatalog.Load()
	if err != nil {
		return releasecatalog.Catalog{}, types.ReleaseArtifactIdentity{}, err
	}
	identity, err := catalog.ResolveTarget(value)
	if err != nil {
		return releasecatalog.Catalog{}, types.ReleaseArtifactIdentity{}, err
	}
	return catalog, identity, nil
}

func writeJSONFile(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary := path + ".temporary"
	if err := os.WriteFile(temporary, payload, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func printJSON(value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(payload))
	return nil
}

func runLabel(prefix string) string {
	return prefix + "-" + time.Now().UTC().Format("20060102T150405.000000000Z")
}

func githubToken(root string, kind string) (string, error) {
	credentials, err := releasecredentials.Load(root)
	if err != nil {
		return "", err
	}
	return credentials.TokenFor(kind)
}

func releaseOutputName(identity types.ReleaseArtifactIdentity) string {
	if identity.Kind == types.ReleaseArtifactKindBox {
		return identity.ID
	}
	return identity.Kind + "-" + identity.ID
}
