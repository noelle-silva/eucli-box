package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestRunWithBuildTimeProducesReproducibleToolBody(t *testing.T) {
	const buildTime = "2026-07-28T07:21:49Z"
	digests := make([]string, 0, 2)
	for index := 0; index < 2; index++ {
		dataDir := filepath.Join(t.TempDir(), "runtime-data")
		if err := run(context.Background(), []string{
			"-tool", "context7",
			"-data-dir", dataDir,
			"-build-time", buildTime,
		}); err != nil {
			t.Fatalf("build reproducible tool body %d: %v", index+1, err)
		}
		bodyDir := filepath.Join(dataDir, "tool-bodies", "context7")
		digest, err := directoryDigest(bodyDir)
		if err != nil {
			t.Fatalf("digest tool body %d: %v", index+1, err)
		}
		digests = append(digests, digest)

		definition := readToolDefinitionFile(t, filepath.Join(bodyDir, "definition.json"))
		expected, _ := time.Parse(time.RFC3339, buildTime)
		if !definition.CreatedAt.Equal(expected) || !definition.UpdatedAt.Equal(expected) {
			t.Fatalf("tool timestamps do not use the explicit build time: created=%s updated=%s", definition.CreatedAt, definition.UpdatedAt)
		}
	}
	if digests[0] != digests[1] {
		t.Fatalf("same source and build time produced different tool bodies: %s != %s", digests[0], digests[1])
	}
}

func TestResolveBuildTimeRejectsInvalidValue(t *testing.T) {
	if _, err := resolveBuildTime("not-a-time"); err == nil {
		t.Fatal("invalid build time was accepted")
	}
}

func directoryDigest(root string) (string, error) {
	records := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		digest := sha256.New()
		size, copyErr := io.Copy(digest, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		records = append(records, fmt.Sprintf("%s\x00%d\x00%s\n", filepath.ToSlash(relative), size, hex.EncodeToString(digest.Sum(nil))))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(records)
	digest := sha256.Sum256([]byte(strings.Join(records, "")))
	return hex.EncodeToString(digest[:]), nil
}
