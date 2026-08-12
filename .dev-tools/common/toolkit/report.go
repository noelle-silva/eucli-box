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

// Check 是单次检查结果。
type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

// Report 是本次运行的总报告，写入产物格。
type Report struct {
	Tool      string    `json:"tool"`
	Version   string    `json:"version,omitempty"`
	RunRoot   string    `json:"runRoot"`
	StartedAt time.Time `json:"startedAt"`
	Status    string    `json:"status"`
	Checks    []Check   `json:"checks"`
	Error     string    `json:"error,omitempty"`
}

// Checker 记录逐项结果并生成报告。
type Checker struct {
	run    *Run
	report Report
	errors []error
}

// NewChecker 为一次运行建立报告器。
func NewChecker(run *Run) *Checker {
	return &Checker{
		run: run,
		report: Report{
			Tool:      run.ToolName,
			Version:   run.Version,
			RunRoot:   run.Root,
			StartedAt: time.Now().UTC(),
			Status:    "running",
			Checks:    []Check{},
		},
	}
}

// Pass 记录一项通过。
func (c *Checker) Pass(name string, summary string) {
	c.report.Checks = append(c.report.Checks, Check{Name: name, Status: "passed", Summary: strings.TrimSpace(summary)})
}

// Fail 记录一项失败。
func (c *Checker) Fail(name string, err error) {
	if err == nil {
		err = fmt.Errorf("未知失败")
	}
	c.report.Checks = append(c.report.Checks, Check{Name: name, Status: "failed", Summary: err.Error()})
	c.errors = append(c.errors, fmt.Errorf("%s：%w", name, err))
}

// Finish 生成最终报告到产物格。
func (c *Checker) Finish() error {
	if combined := errors.Join(c.errors...); combined != nil {
		c.report.Status = "failed"
		c.report.Error = combined.Error()
	} else {
		c.report.Status = "passed"
	}
	payload, err := json.MarshalIndent(c.report, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(filepath.Join(c.run.Output, "report.json"), payload, 0o644)
}
