package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"devtools/common/releasecredentials"
	"devtools/common/toolruntime"
	"eucli-box/pkg/artifactcatalog"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
)

func repositoryRoot(value string) (string, error) {
	return toolruntime.ValidateRepositoryRoot(value)
}

func resolveTarget(value string) (releasecatalog.Sources, types.ReleaseArtifactIdentity, error) {
	sources, err := releasecatalog.LoadSources()
	if err != nil {
		return releasecatalog.Sources{}, types.ReleaseArtifactIdentity{}, err
	}
	roster, err := artifactcatalog.Load()
	if err != nil {
		return releasecatalog.Sources{}, types.ReleaseArtifactIdentity{}, err
	}
	identity, err := roster.ResolveTarget(value)
	if err != nil {
		return releasecatalog.Sources{}, types.ReleaseArtifactIdentity{}, err
	}
	return sources, identity, nil
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

func githubToken(root string, kind string) (string, error) {
	credentials, err := releasecredentials.Load(root)
	if err != nil {
		return "", err
	}
	return credentials.TokenFor(kind)
}

func releaseOutputName(identity types.ReleaseArtifactIdentity) string {
	return identity.Kind + "-" + identity.ID
}
