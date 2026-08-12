package releaseartifact

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

type sourceState struct {
	Record     types.ReleaseSourceRecord
	CommitTime time.Time
}

func readSourceState(ctx context.Context, root string, repository string, verificationOnly bool) (sourceState, error) {
	commit, err := gitOutput(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return sourceState{}, fmt.Errorf("读取源码状态失败：%w", err)
	}
	commit = strings.TrimSpace(commit)
	if len(commit) != 40 {
		return sourceState{}, fmt.Errorf("源码状态没有完整 Git 提交编号")
	}
	commitTimeValue, err := gitOutput(ctx, root, "show", "-s", "--format=%cI", commit)
	if err != nil {
		return sourceState{}, fmt.Errorf("读取源码记录时间失败：%w", err)
	}
	commitTime, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(commitTimeValue))
	if err != nil {
		return sourceState{}, fmt.Errorf("源码记录时间无效：%w", err)
	}
	status, err := gitOutput(ctx, root, "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return sourceState{}, fmt.Errorf("核对源码状态失败：%w", err)
	}
	recorded := strings.TrimSpace(status) == ""
	if !verificationOnly && !recorded {
		return sourceState{}, fmt.Errorf("正式成品只能从已经完整进入 Git 记录的源码状态制作")
	}
	return sourceState{
		Record:     types.ReleaseSourceRecord{Repository: repository, Commit: commit, Recorded: recorded},
		CommitTime: commitTime.UTC(),
	}, nil
}

func gitOutput(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	payload, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w：%s", err, strings.TrimSpace(string(payload)))
	}
	return string(payload), nil
}
