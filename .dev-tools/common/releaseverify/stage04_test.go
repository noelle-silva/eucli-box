package releaseverify

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/workspace"
)

func TestStage04RejectsInvalidMode(t *testing.T) {
	repositoryRoot := t.TempDir()
	runRoot := filepath.Join(workspace.VerificationStageRoot(repositoryRoot, "04"), "run-mode")
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		t.Fatalf("create run root: %v", err)
	}
	if err := Stage04(context.Background(), repositoryRoot, runRoot, "invalid"); err == nil {
		t.Fatal("Stage04() with invalid mode error = nil")
	}
}

func TestStage04RejectsOutsideDirectory(t *testing.T) {
	repositoryRoot := t.TempDir()
	runRoot := filepath.Join(t.TempDir(), "outside")
	if err := Stage04(context.Background(), repositoryRoot, runRoot, "default"); err == nil {
		t.Fatal("Stage04() with outside run root error = nil")
	}
}

func TestStage04RecordsReportForFailedBuild(t *testing.T) {
	repositoryRoot := t.TempDir()
	runRoot := filepath.Join(workspace.VerificationStageRoot(repositoryRoot, "04"), "run-fail")
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		t.Fatalf("create run root: %v", err)
	}
	err := Stage04(context.Background(), repositoryRoot, runRoot, "default")
	if err == nil {
		t.Fatal("Stage04() with empty repository error = nil")
	}
	reportPath := filepath.Join(runRoot, "evidence", "report.json")
	payload, readErr := os.ReadFile(reportPath)
	if readErr != nil {
		t.Fatalf("read report: %v", readErr)
	}
	var report Report
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatalf("parse report: %v", err)
	}
	if report.Stage != "04" || report.Mode != "default" || report.Status != "failed" {
		t.Fatalf("report = %#v", report)
	}
	if len(report.Checks) == 0 {
		t.Fatal("report has no checks")
	}
	if report.Checks[0].Name != "记录真实数据初始状态" {
		t.Fatalf("first check = %#v", report.Checks[0])
	}
	_ = strings.TrimSpace
}
