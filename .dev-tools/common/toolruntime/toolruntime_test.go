package toolruntime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareRunDirRotatesOldRuns(t *testing.T) {
	runtimeRoot := t.TempDir()
	first, err := PrepareRunDir(runtimeRoot, "work", "build")
	if err != nil {
		t.Fatalf("prepare first run: %v", err)
	}
	second, err := PrepareRunDir(runtimeRoot, "work", "build")
	if err != nil {
		t.Fatalf("prepare second run: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(runtimeRoot, "work"))
	if err != nil {
		t.Fatalf("read work: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("work must keep exactly one run, got %d entries", len(entries))
	}
	if entries[0].Name() == filepath.Base(first) || entries[0].Name() != filepath.Base(second) {
		t.Fatalf("work must keep the latest run only, first=%s second=%s entries=%s", filepath.Base(first), filepath.Base(second), entries[0].Name())
	}
	if !strings.HasPrefix(entries[0].Name(), "build-") {
		t.Fatalf("run dir must use given prefix, got %s", entries[0].Name())
	}
}

func TestPrepareRunDirKeepsOtherPrefixes(t *testing.T) {
	runtimeRoot := t.TempDir()
	if _, err := PrepareRunDir(runtimeRoot, "work", "publish"); err != nil {
		t.Fatalf("prepare publish run: %v", err)
	}
	if _, err := PrepareRunDir(runtimeRoot, "work", "build"); err != nil {
		t.Fatalf("prepare build run: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(runtimeRoot, "work"))
	if err != nil {
		t.Fatalf("read work: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("different prefixes must coexist, got %d entries", len(entries))
	}
}

func TestWriteScorecardOverwrites(t *testing.T) {
	runtimeRoot := t.TempDir()
	if err := WriteScorecard(runtimeRoot, "build", map[string]string{"run": "first"}); err != nil {
		t.Fatalf("write first scorecard: %v", err)
	}
	if err := WriteScorecard(runtimeRoot, "build", map[string]string{"run": "second"}); err != nil {
		t.Fatalf("write second scorecard: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(runtimeRoot, "work"))
	if err != nil {
		t.Fatalf("read work: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "build-scorecard.json" {
		t.Fatalf("scorecard must be single stable file, got %v", entries)
	}
	payload, err := os.ReadFile(filepath.Join(runtimeRoot, "work", "build-scorecard.json"))
	if err != nil {
		t.Fatalf("read scorecard: %v", err)
	}
	if !strings.Contains(string(payload), "second") {
		t.Fatalf("scorecard must keep latest content, got %s", payload)
	}
}

func TestValidateWorkLocation(t *testing.T) {
	repo := t.TempDir()
	productRuntime := filepath.Join(repo, ".dev-workspace", ".dev-runtime")
	workspaceDir := filepath.Join(repo, ".dev-workspace")
	sourceArea := filepath.Join(repo, "src")
	outside := filepath.Join(t.TempDir(), "elsewhere")

	cases := []struct {
		name    string
		target  string
		wantErr bool
	}{
		{name: "tool runtime ok", target: filepath.Join(repo, ".dev-workspace", ".dev-tools-runtime", "tool", "work"), wantErr: false},
		{name: "release area ok", target: filepath.Join(repo, ".dev-workspace", ".release", "output"), wantErr: false},
		{name: "outside repo ok", target: outside, wantErr: false},
		{name: "source area rejected", target: filepath.Join(sourceArea, "x"), wantErr: true},
		{name: "product runtime rejected", target: filepath.Join(productRuntime, "eucli-box"), wantErr: true},
		{name: "repo root rejected", target: filepath.Join(repo, "build"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateWorkLocation(repo, tc.target)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %s", tc.target)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %s: %v", tc.target, err)
			}
		})
	}
	_ = workspaceDir
}
