package releaseverify

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

type Cleanup struct {
	Status               string     `json:"status"`
	CompletedDirectories []string   `json:"completedDirectories"`
	PendingDirectories   []string   `json:"pendingDirectories"`
	FinishedAt           *time.Time `json:"finishedAt,omitempty"`
	Message              string     `json:"message,omitempty"`
	Error                string     `json:"error,omitempty"`
}

type Report struct {
	Tool             string     `json:"tool"`
	Mode             string     `json:"mode"`
	RunRoot          string     `json:"runRoot"`
	StartedAt        time.Time  `json:"startedAt"`
	ChecksFinishedAt *time.Time `json:"checksFinishedAt,omitempty"`
	FinishedAt       *time.Time `json:"finishedAt,omitempty"`
	Status           string     `json:"status"`
	Checks           []Check    `json:"checks"`
	Cleanup          Cleanup    `json:"cleanup"`
	Error            string     `json:"error,omitempty"`
}

type recorder struct {
	report Report
	errors []error
}

func newRecorder(tool string, mode string, runRoot string) *recorder {
	return &recorder{report: Report{
		Tool:      tool,
		Mode:      mode,
		RunRoot:   runRoot,
		StartedAt: time.Now().UTC(),
		Status:    "running",
		Checks:    []Check{},
		Cleanup:   newCleanup("not_started", nil, nil),
	}}
}

func (r *recorder) pass(name string, summary string) {
	r.report.Checks = append(r.report.Checks, Check{Name: name, Status: "passed", Summary: strings.TrimSpace(summary)})
}

func (r *recorder) fail(name string, err error) {
	if err == nil {
		err = fmt.Errorf("未知失败")
	}
	r.report.Checks = append(r.report.Checks, Check{Name: name, Status: "failed", Summary: err.Error()})
	r.errors = append(r.errors, fmt.Errorf("%s：%w", name, err))
}

func (r *recorder) finish(paths runPaths) error {
	checksFinishedAt := time.Now().UTC()
	r.report.ChecksFinishedAt = &checksFinishedAt
	reportPath := filepath.Join(paths.evidence, "report.json")
	if checkErr := errors.Join(r.errors...); checkErr != nil {
		return r.finishFailure(paths, "retained", checkErr)
	}

	r.report.Status = "cleanup_pending"
	completed, pending, stateErr := paths.cleanupState()
	if stateErr != nil {
		return r.finishFailure(paths, "failed", fmt.Errorf("确认待清理现场状态失败：%w", stateErr))
	}
	if len(completed) != 0 || !sameStringSequence(pending, disposableDirectories()) {
		return r.finishFailure(paths, "failed", fmt.Errorf("待清理现场不符合外层收尾约定"))
	}

	r.report.Cleanup = newCleanup("pending", completed, pending)
	return writeReport(reportPath, r.report)
}

func (r *recorder) finishFailure(paths runPaths, cleanupStatus string, cause error) error {
	finishedAt := time.Now().UTC()
	completed, pending, stateErr := paths.cleanupState()
	combined := errors.Join(cause, stateErr)
	if stateErr != nil {
		completed = nil
		pending = disposableDirectories()
	}
	r.report.Status = "failed"
	r.report.FinishedAt = &finishedAt
	r.report.Cleanup = newCleanup(cleanupStatus, completed, pending)
	r.report.Cleanup.FinishedAt = &finishedAt
	if cleanupStatus == "failed" {
		r.report.Cleanup.Error = combined.Error()
	}
	r.report.Error = combined.Error()
	return errors.Join(combined, writeReport(filepath.Join(paths.evidence, "report.json"), r.report))
}

func newCleanup(status string, completed []string, pending []string) Cleanup {
	return Cleanup{
		Status:               strings.TrimSpace(status),
		CompletedDirectories: append([]string{}, completed...),
		PendingDirectories:   append([]string{}, pending...),
	}
}

func sameStringSequence(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func writeReport(path string, report Report) error {
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	temporary := path + ".temporary"
	if err := os.WriteFile(temporary, payload, 0o644); err != nil {
		return err
	}
	if err := replaceReportFile(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}
