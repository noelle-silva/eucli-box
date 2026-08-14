package toolkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// VerificationCheck 是验证工具的一次检查结果。
type VerificationCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

// VerificationCleanup 是验证收尾状态，协议与验证收尾脚本一致。
type VerificationCleanup struct {
	Status               string     `json:"status"`
	CompletedDirectories []string   `json:"completedDirectories"`
	PendingDirectories   []string   `json:"pendingDirectories"`
	FinishedAt           *time.Time `json:"finishedAt,omitempty"`
	Message              string     `json:"message,omitempty"`
	Error                string     `json:"error,omitempty"`
}

// VerificationReport 是验证工具的总报告。
type VerificationReport struct {
	Tool             string              `json:"tool"`
	Mode             string              `json:"mode"`
	RunRoot          string              `json:"runRoot"`
	StartedAt        time.Time           `json:"startedAt"`
	ChecksFinishedAt *time.Time          `json:"checksFinishedAt,omitempty"`
	FinishedAt       *time.Time          `json:"finishedAt,omitempty"`
	Status           string              `json:"status"`
	Checks           []VerificationCheck `json:"checks"`
	Cleanup          VerificationCleanup `json:"cleanup"`
	Error            string              `json:"error,omitempty"`
}

// VerificationRecorder 记录逐项检查并生成与收尾脚本协议一致的总报告。
type VerificationRecorder struct {
	report    VerificationReport
	errors    []error
	lastCheck time.Time
}

// NewVerificationRecorder 为一次运行建立报告器。
func NewVerificationRecorder(tool string, mode string, runRoot string) *VerificationRecorder {
	return &VerificationRecorder{
		report: VerificationReport{
			Tool:      tool,
			Mode:      mode,
			RunRoot:   runRoot,
			StartedAt: time.Now().UTC(),
			Status:    "running",
			Checks:    []VerificationCheck{},
			Cleanup:   newVerificationCleanup("not_started", nil, nil),
		},
		lastCheck: time.Now(),
	}
}

// elapsedSinceLast 返回距上一次检查记录的耗时，并刷新计时基准。
func (r *VerificationRecorder) elapsedSinceLast() time.Duration {
	now := time.Now()
	elapsed := now.Sub(r.lastCheck)
	r.lastCheck = now
	return elapsed
}

// Pass 记录一项通过。
func (r *VerificationRecorder) Pass(name string, summary string) {
	r.report.Checks = append(r.report.Checks, VerificationCheck{Name: name, Status: "passed", Summary: strings.TrimSpace(summary)})
	fmt.Printf("[检查] 通过：%s（%s）\n", name, FormatElapsed(r.elapsedSinceLast()))
}

// Fail 记录一项失败。
func (r *VerificationRecorder) Fail(name string, err error) {
	if err == nil {
		err = fmt.Errorf("未知失败")
	}
	r.report.Checks = append(r.report.Checks, VerificationCheck{Name: name, Status: "failed", Summary: err.Error()})
	r.errors = append(r.errors, fmt.Errorf("%s：%w", name, err))
	fmt.Printf("[检查] 失败：%s（%s）\n", name, FormatElapsed(r.elapsedSinceLast()))
}

// FormatElapsed 输出人类可读的耗时文本。
func FormatElapsed(value time.Duration) string {
	rounded := value.Round(time.Millisecond)
	if rounded < time.Second {
		return fmt.Sprintf("%d 毫秒", rounded.Milliseconds())
	}
	return fmt.Sprintf("%.1f 秒", rounded.Seconds())
}

// Finish 写 evidence/report.json 并返回检查是否全部通过。
// disposableDirs 按固定顺序传入本次运行的可清理目录绝对路径。
func (r *VerificationRecorder) Finish(evidenceDir string, disposableDirs []string) error {
	checksFinishedAt := time.Now().UTC()
	r.report.ChecksFinishedAt = &checksFinishedAt
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		return r.finishFailure(evidenceDir, disposableDirs, "failed", fmt.Errorf("建立证据目录失败：%w", err))
	}
	reportPath := filepath.Join(evidenceDir, "report.json")
	if checkErr := errors.Join(r.errors...); checkErr != nil {
		return r.finishFailure(evidenceDir, disposableDirs, "retained", checkErr)
	}
	r.report.Status = "cleanup_pending"
	completed, pending, stateErr := verificationCleanupState(disposableDirs)
	if stateErr != nil {
		return r.finishFailure(evidenceDir, disposableDirs, "failed", fmt.Errorf("确认待清理现场状态失败：%w", stateErr))
	}
	if len(completed) != 0 || !sameStringSequence(pending, verificationDisposableNames()) {
		return r.finishFailure(evidenceDir, disposableDirs, "failed", fmt.Errorf("待清理现场不符合外层收尾约定"))
	}
	r.report.Cleanup = newVerificationCleanup("pending", completed, pending)
	return writeVerificationReport(reportPath, r.report)
}

func (r *VerificationRecorder) finishFailure(evidenceDir string, disposableDirs []string, cleanupStatus string, cause error) error {
	finishedAt := time.Now().UTC()
	completed, pending, stateErr := verificationCleanupState(disposableDirs)
	combined := errors.Join(cause, stateErr)
	if stateErr != nil {
		completed = nil
		pending = verificationDisposableNames()
	}
	r.report.Status = "failed"
	r.report.FinishedAt = &finishedAt
	r.report.Cleanup = newVerificationCleanup(cleanupStatus, completed, pending)
	r.report.Cleanup.FinishedAt = &finishedAt
	if cleanupStatus == "failed" {
		r.report.Cleanup.Error = combined.Error()
	}
	r.report.Error = combined.Error()
	return errors.Join(combined, writeVerificationReport(filepath.Join(evidenceDir, "report.json"), r.report))
}

// verificationDisposableNames 是可清理目录的固定顺序。
func verificationDisposableNames() []string {
	return []string{"inputs", "workspace", "environment", "work", "temp", "cache"}
}

// verificationCleanupState 按固定顺序计算已清理与待清理目录。
func verificationCleanupState(disposableDirs []string) (completed []string, pending []string, stateErr error) {
	names := verificationDisposableNames()
	if len(disposableDirs) != len(names) {
		return nil, nil, fmt.Errorf("可清理目录数量与收尾约定不符")
	}
	for index, dir := range disposableDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			completed = append(completed, names[index])
		} else if err != nil {
			pending = append(pending, names[index:]...)
			return completed, pending, fmt.Errorf("读取清理目录 %s 失败：%w", names[index], err)
		} else {
			pending = append(pending, names[index])
		}
	}
	return completed, pending, nil
}

func newVerificationCleanup(status string, completed []string, pending []string) VerificationCleanup {
	return VerificationCleanup{
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

func writeVerificationReport(path string, report VerificationReport) error {
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
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
