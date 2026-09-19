package release

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

const operationSchemaVersion = 1

const (
	OperationActionInstall = "install"
	OperationActionUpdate  = "update"

	OperationResultRunning = "running"
	OperationResultSuccess = "success"
	OperationResultFailed  = "failed"
	OperationResultCancelled = "cancelled"
)

// OperationRecord 记录一次单项安装或更新操作停在哪一个环节。
// Phase 只能取固定 Phase 词表；Result 单独表示操作是否已经结束以及结束结果。
type OperationRecord struct {
	SchemaVersion  int                         `json:"schemaVersion"`
	OperationID    string                      `json:"operationId"`
	Artifact       types.ReleaseArtifactIdentity `json:"artifact"`
	Action         string                      `json:"action"`
	TargetVersion  string                      `json:"targetVersion"`
	Phase          string                      `json:"phase"`
	Result         string                      `json:"result"`
	CurrentVersion string                      `json:"currentVersion"`
	WorkDirectory  string                      `json:"workDirectory"`
	StartedAt      time.Time                   `json:"startedAt"`
	UpdatedAt      time.Time                   `json:"updatedAt"`
	ErrorCode      string                      `json:"errorCode"`
	ErrorMessage   string                      `json:"errorMessage"`
}

// NewOperationRecord 构造一次新的操作记录；操作 ID、身份、动作、目标版本和工作目录全部必须有效。
func NewOperationRecord(operationID string, artifact types.ReleaseArtifactIdentity, action string, targetVersion string, workDirectory string) (OperationRecord, error) {
	if err := ValidateOperationID(operationID); err != nil {
		return OperationRecord{}, err
	}
	if err := validateIdentity(artifact); err != nil {
		return OperationRecord{}, err
	}
	if action != OperationActionInstall && action != OperationActionUpdate {
		return OperationRecord{}, fmt.Errorf("操作动作无效：%s", action)
	}
	if err := ValidateVersion(targetVersion); err != nil {
		return OperationRecord{}, fmt.Errorf("目标版本无效：%w", err)
	}
	if strings.TrimSpace(workDirectory) == "" {
		return OperationRecord{}, fmt.Errorf("工作目录不能为空")
	}
	now := time.Now().UTC()
	return OperationRecord{
		SchemaVersion: operationSchemaVersion,
		OperationID:   operationID,
		Artifact:      artifact,
		Action:        action,
		TargetVersion: targetVersion,
		Phase:         types.ArtifactPhaseCandidate,
		Result:        OperationResultRunning,
		WorkDirectory: workDirectory,
		StartedAt:     now,
		UpdatedAt:     now,
	}, nil
}

// ValidateOperationID 校验操作 ID 格式；拒绝路径分隔符和空值。
func ValidateOperationID(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, `/\\`) || value == "." || value == ".." {
		return fmt.Errorf("操作 ID 无效")
	}
	return nil
}

// WriteOperationRecord 把操作记录写入临时文件后原子替换。
func WriteOperationRecord(path string, record OperationRecord) error {
	if err := validateOperationRecord(record); err != nil {
		return err
	}
	return writeJSONAtomic(path, record)
}

// ReadOperationRecord 严格读取并校验操作记录；文件不存在返回 os.ErrNotExist 的包装错误。
func ReadOperationRecord(path string) (OperationRecord, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return OperationRecord{}, err
	}
	var record OperationRecord
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return OperationRecord{}, fmt.Errorf("操作记录无效：%w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return OperationRecord{}, fmt.Errorf("操作记录包含多余内容")
		}
		return OperationRecord{}, err
	}
	if err := validateOperationRecord(record); err != nil {
		return OperationRecord{}, err
	}
	return record, nil
}

// OperationPhaseIsPreSwitch 表示中断发生在切换之前：当前版本不变，只删除未核对临时内容。
func OperationPhaseIsPreSwitch(phase string) bool {
	switch phase {
	case types.ArtifactPhaseCandidate, types.ArtifactPhaseCompatibility, types.ArtifactPhaseActivity,
		types.ArtifactPhaseDownload, types.ArtifactPhaseManifest, types.ArtifactPhaseArchive,
		types.ArtifactPhasePackage, types.ArtifactPhasePrepare:
		return true
	default:
		return false
	}
}

// OperationPhaseIsPostSwitch 表示中断发生在切换或启动验收之后：必须核对当前版本记录并恢复。
func OperationPhaseIsPostSwitch(phase string) bool {
	switch phase {
	case types.ArtifactPhaseSwitch, types.ArtifactPhaseProbe, types.ArtifactPhaseRestore, types.ArtifactPhaseRefresh:
		return true
	default:
		return false
	}
}

// OperationPhaseAllowsCancel 表示该阶段允许取消：切换前阶段与探测阶段。
// 进入切换后当前版本开始变更，必须保证要么切成功、要么完整回滚，不允许中途打断。
func OperationPhaseAllowsCancel(phase string) bool {
	if OperationPhaseIsPreSwitch(phase) {
		return true
	}
	return phase == types.ArtifactPhaseProbe
}

// IsTerminal 表示该操作记录已经结束（失败或取消），不再是进行中的操作。
func (r OperationRecord) IsTerminal() bool {
	return r.Result == OperationResultFailed || r.Result == OperationResultCancelled
}

// RunningOperationSnapshot 是一次进行中的安装/更新任务对外的事实快照：
// 任务基座（当前版本事实）在受理时刻固定，阶段与下载进度随执行推进。
// 运行中查询只依赖它，不再回读磁盘，避免与原子替换竞争。
type RunningOperationSnapshot struct {
	OperationID    string
	Phase          string
	Progress       types.ReleaseOperationProgress
	CurrentVersion string
	Installed      bool
}

func validateOperationRecord(record OperationRecord) error {
	if record.SchemaVersion != operationSchemaVersion {
		return fmt.Errorf("操作记录 schemaVersion 必须为 %d", operationSchemaVersion)
	}
	if err := ValidateOperationID(record.OperationID); err != nil {
		return err
	}
	if err := validateIdentity(record.Artifact); err != nil {
		return err
	}
	if record.Action != OperationActionInstall && record.Action != OperationActionUpdate {
		return fmt.Errorf("操作记录动作无效")
	}
	if err := ValidateVersion(record.TargetVersion); err != nil {
		return fmt.Errorf("操作记录目标版本无效：%w", err)
	}
	if !validArtifactPhase(record.Phase) {
		return fmt.Errorf("操作记录阶段无效：%s", record.Phase)
	}
	if record.Result != OperationResultRunning && record.Result != OperationResultSuccess && record.Result != OperationResultFailed && record.Result != OperationResultCancelled {
		return fmt.Errorf("操作记录结果无效：%s", record.Result)
	}
	if record.CurrentVersion != "" && ValidateVersion(record.CurrentVersion) != nil {
		return fmt.Errorf("操作记录当前版本无效：%s", record.CurrentVersion)
	}
	if strings.TrimSpace(record.WorkDirectory) == "" {
		return fmt.Errorf("操作记录工作目录不能为空")
	}
	if record.StartedAt.IsZero() || record.UpdatedAt.IsZero() {
		return fmt.Errorf("操作记录时间不能为空")
	}
	return nil
}

func validArtifactPhase(phase string) bool {
	switch phase {
	case types.ArtifactPhaseCandidate, types.ArtifactPhaseCompatibility, types.ArtifactPhaseActivity,
		types.ArtifactPhaseDownload, types.ArtifactPhaseManifest, types.ArtifactPhaseArchive,
		types.ArtifactPhasePackage, types.ArtifactPhasePrepare, types.ArtifactPhaseSwitch,
		types.ArtifactPhaseProbe, types.ArtifactPhaseRestore, types.ArtifactPhaseRefresh:
		return true
	default:
		return false
	}
}
