package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

// updateCandidate 读取业务端候选；没有新版时由调用方通过版本比较判断。
func (m *localBoxManager) updateCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*releasecheck.ReleaseCandidate, error) {
	return m.source.LatestCandidate(ctx, identity)
}

// requestBoxActiveWork 只读查询业务端未结束的真实工作数量；
// 查询失败时返回错误，程序更换不得进入下载。
func requestBoxActiveWork(ctx context.Context, connection *boxConnection) (int, error) {
	target, err := url.Parse(strings.TrimRight(connection.BaseURL, "/") + "/api/box/active-work")
	if err != nil {
		return 0, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Authorization", "Bearer "+connection.Credential)
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return 0, fmt.Errorf("查询真实工作状态失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("查询真实工作状态返回 %d", response.StatusCode)
	}
	var envelope struct {
		Data struct {
			ActiveWork []json.RawMessage `json:"activeWork"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return 0, fmt.Errorf("真实工作状态资料无效：%w", err)
	}
	return len(envelope.Data.ActiveWork), nil
}

// requestBoxShutdown 通过受托网关要求业务端正常退出，不确认未结束工作。
// 返回 409 表示业务端存在未结束的真实工作。
func requestBoxShutdown(ctx context.Context, connection *boxConnection) (int, error) {
	target, err := url.Parse(strings.TrimRight(connection.BaseURL, "/") + "/api/box/shutdown")
	if err != nil {
		return 0, err
	}
	payload, _ := json.Marshal(map[string]bool{"confirm": false})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), strings.NewReader(string(payload)))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Authorization", "Bearer "+connection.Credential)
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return 0, fmt.Errorf("请求业务端停止失败：%w", err)
	}
	defer response.Body.Close()
	return response.StatusCode, nil
}

func (m *localBoxManager) operationRecordPath() string {
	return filepath.Join(m.paths.programStoreDir, "operation.json")
}

// recordOperationFailure 把操作记录标记为失败并保留失败原因；读取或写入失败不阻断现场。
func (m *localBoxManager) recordOperationFailure(phase string, code string, message string) {
	record, err := release.ReadOperationRecord(m.operationRecordPath())
	if err != nil {
		return
	}
	record.Phase = phase
	record.Result = release.OperationResultFailed
	record.ErrorCode = code
	record.ErrorMessage = message
	record.UpdatedAt = time.Now().UTC()
	_ = release.WriteOperationRecord(m.operationRecordPath(), record)
}

// failUpdateBeforeSwitch 是切换前失败的统一收口：
// operation.json 记 failed、删除本次工作区、保持当前版本，不进入切换。
func (m *localBoxManager) failUpdateBeforeSwitch(state localBoxState, workDir string, code string, message string, phase string) (localBoxState, error) {
	m.recordOperationFailure(phase, code, message)
	_ = os.RemoveAll(workDir)
	failed := localBoxFailure(state, code, message, phase)
	m.publishState(failed)
	return failed, nil
}

// update 是业务端更新的完整执行主流程，只凭用户一次点击连续完成：
// 活动保护 → 下载核对 → 准备新版 → 停止旧版 → 一次性切换 → 启动验收。
func (m *localBoxManager) update(ctx context.Context, client clientRelease) (localBoxState, error) {
	m.operation.Lock()
	defer m.operation.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	// 1. 处理上次中断遗留
	if err := m.recoverPendingUpdate(ctx); err != nil {
		state := localBoxFailure(initialLocalBoxState(), localBoxErrorCodeFrom(err, localBoxErrorUpdateFailed), err.Error(), localBoxStatusNotInstalled)
		m.publishState(state)
		return state, nil
	}
	// 2. 读取当前安装事实
	record, err := m.readInstall()
	if err != nil {
		state := localBoxFailure(initialLocalBoxState(), localBoxErrorUpdateFailed, err.Error(), localBoxStatusNotInstalled)
		m.publishState(state)
		return state, nil
	}
	if record == nil {
		state := localBoxFailure(initialLocalBoxState(), localBoxErrorUpdateFailed, "业务端尚未安装，更新只针对已安装业务端", localBoxStatusNotInstalled)
		m.publishState(state)
		return state, nil
	}
	current, err := m.currentProgramFacts()
	if err != nil {
		state := localBoxFailure(initialLocalBoxState(), localBoxErrorUpdateFailed, "程序资料不完整，需要重新安装", localBoxStatusStopped)
		m.publishState(state)
		return state, nil
	}
	state := installStateFor(record, current)
	identity := types.ReleaseArtifactIdentity{Kind: localBoxArtifactID, ID: localBoxArtifactID}

	// 3. 读取候选；没有新版或候选不高于当前版本时直接返回当前状态
	if m.source == nil {
		state = localBoxFailure(state, "LOCAL_BOX_RELEASE_UNAVAILABLE", "业务端候选读取能力未初始化", localBoxStatusStopped)
		m.publishState(state)
		return state, nil
	}
	candidate, err := m.updateCandidate(ctx, identity)
	if err != nil {
		state = localBoxFailure(state, localBoxErrorCodeFrom(err, "LOCAL_BOX_RELEASE_UNAVAILABLE"), err.Error(), localBoxStatusStopped)
		m.publishState(state)
		return state, nil
	}
	if candidate == nil {
		return state, nil
	}
	comparison, err := release.CompareVersions(candidate.Version, current.Version)
	if err != nil {
		state = localBoxFailure(state, localBoxErrorUpdateFailed, err.Error(), localBoxStatusStopped)
		m.publishState(state)
		return state, nil
	}
	if comparison <= 0 {
		return state, nil
	}

	// 4. 客户端适用关系只说明不阻止：适用结果由界面按既有 EucliBoxCompatibility 事实展示

	// 5. 活动保护（下载前）
	if connection := m.currentConnection(); connection != nil {
		activeCount, err := requestBoxActiveWork(ctx, connection)
		if err != nil {
			state = localBoxFailure(state, localBoxErrorUpdateBlocked, fmt.Sprintf("无法确认真实工作状态：%v", err), localBoxStatusStopped)
			m.publishState(state)
			return state, nil
		}
		if activeCount > 0 {
			state = localBoxFailure(state, localBoxErrorUpdateBlocked, fmt.Sprintf("业务端有 %d 项真实工作尚未结束，需要先等待或停止这些工作", activeCount), localBoxStatusStopped)
			m.publishState(state)
			return state, nil
		}
	}

	// 6. 建立更新工作区与操作记录
	operationID := time.Now().UTC().Format("20060102T150405.000000000Z")
	workDir := filepath.Join(m.paths.workDir, "update-"+operationID)
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return m.failUpdateBeforeSwitch(state, workDir, localBoxErrorUpdateFailed, err.Error(), types.ArtifactPhaseCandidate)
	}
	operation, err := release.NewOperationRecord(operationID, identity, release.OperationActionUpdate, candidate.Version, workDir)
	if err != nil {
		return m.failUpdateBeforeSwitch(state, workDir, localBoxErrorUpdateFailed, err.Error(), types.ArtifactPhaseCandidate)
	}
	operation.CurrentVersion = current.Version
	if err := release.WriteOperationRecord(m.operationRecordPath(), operation); err != nil {
		return m.failUpdateBeforeSwitch(state, workDir, localBoxErrorUpdateFailed, err.Error(), types.ArtifactPhaseCandidate)
	}
	store, err := m.programStore()
	if err != nil {
		return m.failUpdateBeforeSwitch(state, workDir, localBoxErrorUpdateFailed, err.Error(), types.ArtifactPhaseCandidate)
	}
	state.TargetVersion = candidate.Version
	m.publishState(state)

	// 7. 下载与核对
	state.Status = localBoxStatusDownloading
	state.Progress = localBoxProgress{Phase: types.ArtifactPhaseDownload, TotalBytes: candidate.SizeBytes}
	m.publishState(state)
	validated, nextState, err := m.downloadAndVerify(ctx, candidate, workDir, state)
	if err != nil {
		return m.failUpdateBeforeSwitch(state, workDir, localBoxErrorCodeFrom(err, "LOCAL_BOX_PACKAGE_INVALID"), err.Error(), types.ArtifactPhaseDownload)
	}
	state = nextState

	// 8. 准备新版
	state.Status = localBoxStatusInstalling
	state.Progress.Phase = types.ArtifactPhasePrepare
	m.publishState(state)
	prepared, err := store.PrepareVersion(ctx, validated.Directory, validated.Product, validated.Files)
	if err != nil {
		return m.failUpdateBeforeSwitch(state, workDir, localBoxErrorUpdateFailed, fmt.Sprintf("准备新版业务端失败：%v", err), types.ArtifactPhasePrepare)
	}

	// 9. 停止旧版
	state.Status = localBoxStatusWaitingStop
	state.Progress.Phase = types.ArtifactPhaseActivity
	m.publishState(state)
	if process := m.currentProcess(); process != nil {
		statusCode, err := requestBoxShutdown(ctx, process.connection)
		if err != nil {
			return m.failUpdateBeforeSwitch(state, workDir, localBoxErrorUpdateBlocked, fmt.Sprintf("请求旧业务端停止失败：%v", err), types.ArtifactPhaseActivity)
		}
		if statusCode == http.StatusConflict {
			return m.failUpdateBeforeSwitch(state, workDir, localBoxErrorUpdateBlocked, "下载期间出现了新的真实工作，需要先等待或停止这些工作", types.ArtifactPhaseActivity)
		}
		if statusCode < 200 || statusCode >= 300 {
			return m.failUpdateBeforeSwitch(state, workDir, "LOCAL_BOX_STOP_FAILED", fmt.Sprintf("业务端停止请求返回 %d", statusCode), types.ArtifactPhaseActivity)
		}
		stoppedState, err := m.waitStoppedAndClear(ctx, process, state)
		if err != nil {
			return m.failUpdateBeforeSwitch(state, workDir, localBoxErrorCodeFrom(err, "LOCAL_BOX_NOT_STOPPED"), err.Error(), types.ArtifactPhaseActivity)
		}
		state = stoppedState
	}

	// 10. 一次性切换
	state.Status = localBoxStatusSwitching
	state.Progress.Phase = types.ArtifactPhaseSwitch
	m.publishState(state)
	if err := store.Activate(ctx, prepared, current.Version); err != nil {
		if restoreErr := store.Restore(ctx, current.Version); restoreErr != nil {
			return m.handleUnsafeUpdateFailure(state, workDir, fmt.Sprintf("切换失败且恢复上一版也失败：%v（恢复错误：%v）", err, restoreErr))
		}
		return m.failUpdateBeforeSwitch(state, workDir, localBoxErrorUpdateFailed, fmt.Sprintf("切换业务端版本失败：%v", err), types.ArtifactPhaseSwitch)
	}

	// 11. 启动验收：与阶段三是同一条验收链（ready 解析、四事实核对、登记核对、verifyLocalRunFacts）
	state.Status = localBoxStatusStarting
	state.Progress.Phase = types.ArtifactPhaseProbe
	m.publishState(state)
	program, err := m.currentProgramFacts()
	if err != nil {
		return m.handleUpdateProbeFailure(ctx, state, workDir, current.Version, err)
	}
	process, err := startLocalBoxProcess(ctx, m.paths, *record, program)
	if err != nil {
		return m.handleUpdateProbeFailure(ctx, state, workDir, current.Version, err)
	}
	return m.finishUpdateSuccess(state, process, program.Version, false, true, workDir), nil
}
