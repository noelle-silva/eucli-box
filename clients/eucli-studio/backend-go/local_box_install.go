package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"eucli-box/pkg/localrun"
	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

type localBoxPaths struct {
	clientDataDir    string
	rootDir          string
	installPath      string
	programStoreDir  string
	dataDir          string
	runtimeDir       string
	registrationPath string
	workDir          string
}

func newLocalBoxPaths(clientDataDir string) (localBoxPaths, error) {
	return newLocalBoxPathsWithRoot(clientDataDir, "")
}

// newLocalBoxPathsWithRoot 计算客户端本地业务端资料路径。
// boxRoot 为空时业务端资料位于客户端数据目录下（正式模式）；
// boxRoot 非空时业务端资料全部位于开发资料根（开发模式）。
func newLocalBoxPathsWithRoot(clientDataDir string, boxRoot string) (localBoxPaths, error) {
	clientDataDir = strings.TrimSpace(clientDataDir)
	if clientDataDir == "" {
		return localBoxPaths{}, errors.New("FW_APP_DATA_DIR is required")
	}
	absolute, err := filepath.Abs(clientDataDir)
	if err != nil {
		return localBoxPaths{}, fmt.Errorf("解析客户端数据目录失败：%w", err)
	}
	root := filepath.Join(filepath.Clean(absolute), "local-eucli-box")
	if rootValue := strings.TrimSpace(boxRoot); rootValue != "" {
		root, err = filepath.Abs(rootValue)
		if err != nil {
			return localBoxPaths{}, fmt.Errorf("解析开发业务端资料根失败：%w", err)
		}
		root = filepath.Clean(root)
	}
	runtimeDir := filepath.Join(root, "runtime")
	return localBoxPaths{
		clientDataDir:    filepath.Clean(absolute),
		rootDir:          root,
		installPath:      filepath.Join(root, "install.json"),
		programStoreDir:  filepath.Join(root, "program", "eucli-box"),
		dataDir:          filepath.Join(root, "data"),
		runtimeDir:       runtimeDir,
		registrationPath: filepath.Join(runtimeDir, "registration.json"),
		workDir:          filepath.Join(root, "work"),
	}, nil
}

// localBoxInstallRecord 是当前版本为 2 的安装记录：
// 当前版本与程序目录不再保存在记录中，由程序版本仓库的 current.json 单独说真话。
type localBoxInstallRecord struct {
	SchemaVersion   int                           `json:"schemaVersion"`
	Artifact        types.ReleaseArtifactIdentity `json:"artifact"`
	Source          string                        `json:"source,omitempty"`
	Platform        string                        `json:"platform"`
	InstallIdentity string                        `json:"installIdentity"`
	DataIdentity    string                        `json:"dataIdentity"`
	DataDir         string                        `json:"dataDir"`
	RuntimeDir      string                        `json:"runtimeDir"`
}

type localBoxInstallRecordError struct {
	err error
}

func (e localBoxInstallRecordError) Error() string { return e.err.Error() }
func (e localBoxInstallRecordError) Unwrap() error { return e.err }

type localBoxManager struct {
	paths        localBoxPaths
	source       localBoxArtifactSource
	mu           sync.Mutex
	operation    sync.Mutex
	state        localBoxState
	process      *localBoxProcess
	connection   *boxConnection
	onState      func(localBoxState)
	onConnect    func(*boxConnection)
	onDisconnect func()
}

func newLocalBoxManager(paths localBoxPaths, source localBoxArtifactSource, onState func(localBoxState), onConnect func(*boxConnection), onDisconnect func()) *localBoxManager {
	manager := &localBoxManager{
		paths: paths, state: initialLocalBoxState(),
		onState: onState, onConnect: onConnect, onDisconnect: onDisconnect,
	}
	if source != nil {
		manager.source = source
	}
	return manager
}

func (m *localBoxManager) sourceKind() localBoxSourceKind {
	if m.source == nil {
		return localBoxSourceOfficial
	}
	return m.source.Kind().normalize()
}

// detach 客户端真正退出但业务端选择后台继续运行时调用：
// 只清理客户端连接状态，不请求业务端停止、不等待进程结束、不清理登记。
func (m *localBoxManager) detach(ctx context.Context) {
	m.mu.Lock()
	process := m.process
	m.process = nil
	m.connection = nil
	onDisconnect := m.onDisconnect
	m.mu.Unlock()
	if onDisconnect != nil {
		onDisconnect()
	}
	if process != nil && !process.reconnected {
		process.cleanup()
	}
}

// restart 重新启动业务端：先让当前业务端正常退出并等待真实结束，再启动新业务端。
func (m *localBoxManager) restart(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	state, err := m.stop(ctx)
	if err != nil && state.Status != localBoxStatusStopped {
		return err
	}
	record, err := m.readInstall()
	if err != nil {
		return err
	}
	if record == nil {
		return newError("LOCAL_BOX_NOT_INSTALLED", "业务端尚未安装")
	}
	_, err = m.start(ctx)
	return err
}

func (m *localBoxManager) currentState() localBoxState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

func (m *localBoxManager) publishState(state localBoxState) {
	state.Source = string(m.sourceKind())
	m.mu.Lock()
	m.state = state
	onState := m.onState
	m.mu.Unlock()
	if onState != nil {
		onState(state)
	}
}

func (m *localBoxManager) setConnection(connection *boxConnection) {
	m.mu.Lock()
	m.connection = connection
	onConnect := m.onConnect
	m.mu.Unlock()
	if onConnect != nil {
		onConnect(connection)
	}
}

func (m *localBoxManager) clearConnection() {
	m.mu.Lock()
	wasConnected := m.connection != nil
	m.connection = nil
	onDisconnect := m.onDisconnect
	m.mu.Unlock()
	if wasConnected && onDisconnect != nil {
		onDisconnect()
	}
}

func (m *localBoxManager) readInstall() (*localBoxInstallRecord, error) {
	var record localBoxInstallRecord
	if err := localrun.ReadStrictJSON(m.paths.installPath, &record); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, localBoxInstallRecordError{err: fmt.Errorf("LOCAL_BOX_INSTALL_FAILED: 安装资料损坏：%w", err)}
	}
	if err := validateLocalBoxInstallRecord(record, m.paths); err != nil {
		if isLocalBoxLegacyLayoutError(err) {
			return nil, localBoxInstallRecordError{err: err}
		}
		return nil, localBoxInstallRecordError{err: fmt.Errorf("LOCAL_BOX_INSTALL_FAILED: 安装资料无效：%w", err)}
	}
	return &record, nil
}

// isLocalBoxLegacyLayoutError 判断错误是否来自旧版平铺安装布局（v1 安装记录）。
func isLocalBoxLegacyLayoutError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "LOCAL_BOX_LEGACY_LAYOUT")
}

// isLocalBoxLegacyLayout 判断读取安装记录返回的错误是否属于旧版平铺布局。
func isLocalBoxLegacyLayout(err error) bool {
	var target localBoxInstallRecordError
	return errors.As(err, &target) && isLocalBoxLegacyLayoutError(target.Unwrap())
}

func validateLocalBoxInstallRecord(record localBoxInstallRecord, paths localBoxPaths) error {
	if record.SchemaVersion != 2 {
		if record.SchemaVersion == 1 {
			return errors.New("LOCAL_BOX_LEGACY_LAYOUT: 旧版平铺安装布局，需要重新安装")
		}
		return errors.New("安装资料身份无效")
	}
	if record.Artifact.Kind != localBoxArtifactID || record.Artifact.ID != localBoxArtifactID {
		return errors.New("安装资料身份无效")
	}
	if record.Source != "" && !localBoxSourceKind(record.Source).valid() {
		return errors.New("安装资料来源无效")
	}
	if record.Platform != types.ReleasePlatformWindowsX64 {
		return errors.New("安装资料平台无效")
	}
	if !filepath.IsAbs(record.DataDir) || !filepath.IsAbs(record.RuntimeDir) {
		return errors.New("安装资料目录必须使用绝对路径")
	}
	if err := localrun.ValidateIdentity(record.InstallIdentity, localrun.IdentityKindInstall); err != nil {
		return err
	}
	if err := localrun.ValidateIdentity(record.DataIdentity, localrun.IdentityKindData); err != nil {
		return err
	}
	for _, pair := range [][2]string{{record.DataDir, paths.dataDir}, {record.RuntimeDir, paths.runtimeDir}} {
		actual, err := filepath.Abs(strings.TrimSpace(pair[0]))
		if err != nil || filepath.Clean(actual) != filepath.Clean(pair[1]) {
			return errors.New("安装资料目录事实无效")
		}
	}
	return nil
}

func (m *localBoxManager) writeInstall(record localBoxInstallRecord) error {
	if err := validateLocalBoxInstallRecord(record, m.paths); err != nil {
		return err
	}
	return localrun.WritePrivateJSON(m.paths.installPath, record)
}

func isLocalBoxInstallRecordInvalid(err error) bool {
	var target localBoxInstallRecordError
	return errors.As(err, &target)
}

// programStore 返回业务端程序版本仓库；根目录名必须等于发布物 ID。
func (m *localBoxManager) programStore() (release.ProgramStore, error) {
	return release.NewProgramStore(m.paths.programStoreDir,
		types.ReleaseArtifactIdentity{Kind: localBoxArtifactID, ID: localBoxArtifactID})
}

// currentProgramFacts 读取当前版本事实。
// install.json 为 v2 且存在、但 current.json 缺失或核对失败时返回错误，不猜测版本。
func (m *localBoxManager) currentProgramFacts() (release.CurrentProgram, error) {
	store, err := m.programStore()
	if err != nil {
		return release.CurrentProgram{}, err
	}
	return store.Current()
}

func (m *localBoxManager) recoverCorruptInstallRecord() error {
	info, err := os.Stat(m.paths.programStoreDir)
	if errors.Is(err, os.ErrNotExist) {
		return os.Remove(m.paths.installPath)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("损坏安装资料对应的程序目录不是目录")
	}
	entries, err := os.ReadDir(m.paths.programStoreDir)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("损坏安装资料对应的程序目录包含未知内容，拒绝自动覆盖")
	}
	if err := os.Remove(m.paths.programStoreDir); err != nil {
		return err
	}
	return os.Remove(m.paths.installPath)
}

func installStateFor(record *localBoxInstallRecord, current release.CurrentProgram) localBoxState {
	state := initialLocalBoxState()
	if record == nil {
		return state
	}
	state.Installed = true
	state.CurrentVersion = current.Version
	state.Status = localBoxStatusStopped
	return state
}

func (m *localBoxManager) currentConnection() *boxConnection {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.connection == nil {
		return nil
	}
	copy := *m.connection
	return &copy
}

func (m *localBoxManager) status(ctx context.Context) (localBoxState, error) {
	m.operation.Lock()
	err := m.recoverPendingUpdate(ctx)
	m.operation.Unlock()
	if err != nil {
		state := localBoxFailure(initialLocalBoxState(), localBoxErrorCodeFrom(err, localBoxErrorUpdateFailed), err.Error(), localBoxStatusNotInstalled)
		m.publishState(state)
		return state, nil
	}
	record, err := m.readInstall()
	if err != nil {
		if isLocalBoxLegacyLayout(err) {
			state := localBoxFailure(initialLocalBoxState(), "LOCAL_BOX_LEGACY_LAYOUT", "旧版平铺安装布局，需要重新安装；数据不受影响", localBoxStatusNotInstalled)
			m.publishState(state)
			return state, nil
		}
		state := localBoxFailure(initialLocalBoxState(), "LOCAL_BOX_INSTALL_FAILED", err.Error(), localBoxStatusNotInstalled)
		m.publishState(state)
		return state, nil
	}
	state := initialLocalBoxState()
	if record == nil {
		m.publishState(state)
		return state, nil
	}
	current, err := m.currentProgramFacts()
	if err != nil {
		state = localBoxFailure(state, "LOCAL_BOX_INSTALL_FAILED", "程序资料不完整，需要重新安装", localBoxStatusStopped)
		m.publishState(state)
		return state, nil
	}
	state = installStateFor(record, current)
	if m.recordSourceMismatch(record) {
		state = localBoxFailure(state, "LOCAL_BOX_SOURCE_MISMATCH", "已安装业务端与当前成品来源不匹配，需要重新安装", localBoxStatusStopped)
		m.publishState(state)
		return state, nil
	}
	if process := m.currentProcess(); process != nil {
		state.Status = localBoxStatusConnected
		state.Connected = true
		m.publishState(state)
		return state, nil
	}
	return m.start(ctx)
}

func (m *localBoxManager) install(ctx context.Context) (localBoxState, error) {
	m.operation.Lock()
	defer m.operation.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	if err := m.recoverPendingUpdate(ctx); err != nil {
		state := localBoxFailure(initialLocalBoxState(), localBoxErrorCodeFrom(err, localBoxErrorUpdateFailed), err.Error(), localBoxStatusNotInstalled)
		m.publishState(state)
		return state, nil
	}
	if m.currentProcess() != nil {
		return m.currentState(), newError("LOCAL_BOX_INSTALL_FAILED", "业务端正在运行，不能重复安装")
	}
	if record, err := m.readInstall(); err != nil {
		if isLocalBoxLegacyLayout(err) {
			if recoverErr := m.removeLegacyProgramLayout(); recoverErr != nil {
				state := localBoxFailure(initialLocalBoxState(), "LOCAL_BOX_INSTALL_FAILED", recoverErr.Error(), localBoxStatusInstalling)
				m.publishState(state)
				return state, nil
			}
		} else if isLocalBoxInstallRecordInvalid(err) {
			if recoverErr := m.recoverCorruptInstallRecord(); recoverErr != nil {
				state := localBoxFailure(initialLocalBoxState(), "LOCAL_BOX_INSTALL_FAILED", recoverErr.Error(), localBoxStatusInstalling)
				m.publishState(state)
				return state, nil
			}
		} else {
			state := localBoxFailure(initialLocalBoxState(), "LOCAL_BOX_INSTALL_FAILED", err.Error(), localBoxStatusNotInstalled)
			m.publishState(state)
			return state, nil
		}
	} else if record != nil {
		if !m.recordSourceMismatch(record) {
			current, factsErr := m.currentProgramFacts()
			if factsErr != nil {
				state := localBoxFailure(initialLocalBoxState(), "LOCAL_BOX_INSTALL_FAILED", "程序资料不完整，需要重新安装", localBoxStatusInstalling)
				m.publishState(state)
				return state, nil
			}
			state := installStateFor(record, current)
			m.publishState(state)
			return state, newError("LOCAL_BOX_INSTALL_FAILED", "业务端已经安装")
		}
		if err := m.prepareReinstall(record); err != nil {
			state := localBoxFailure(initialLocalBoxState(), "LOCAL_BOX_INSTALL_FAILED", err.Error(), localBoxStatusInstalling)
			m.publishState(state)
			return state, nil
		}
	}
	if m.source == nil {
		state := localBoxFailure(initialLocalBoxState(), "LOCAL_BOX_RELEASE_UNAVAILABLE", "业务端候选读取能力未初始化", localBoxStatusCheckingRelease)
		m.publishState(state)
		return state, nil
	}
	state := initialLocalBoxState()
	state.Status = localBoxStatusCheckingRelease
	m.publishState(state)
	candidate, err := m.source.LatestCandidate(ctx, types.ReleaseArtifactIdentity{Kind: localBoxArtifactID, ID: localBoxArtifactID})
	if err != nil {
		state = localBoxFailure(state, localBoxErrorCodeFrom(err, "LOCAL_BOX_RELEASE_UNAVAILABLE"), err.Error(), localBoxStatusCheckingRelease)
		m.publishState(state)
		return state, nil
	}
	if candidate == nil {
		state = localBoxFailure(state, "LOCAL_BOX_RELEASE_UNAVAILABLE", "当前来源没有可安装的业务端成品", localBoxStatusCheckingRelease)
		m.publishState(state)
		return state, nil
	}
	state.LatestVersion = candidate.Version
	state.ReleaseNotes = candidate.ReleaseNotes
	state.DownloadSize = candidate.SizeBytes
	state.Status = localBoxStatusReadyToInstall
	m.publishState(state)

	runIdentity, err := localrun.NewIdentity(localrun.IdentityKindRun)
	if err != nil {
		return m.installFailure(state, "LOCAL_BOX_INSTALL_FAILED", err.Error(), localBoxStatusReadyToInstall)
	}
	workDir := filepath.Join(m.paths.workDir, "install-"+runIdentity)
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return m.installFailure(state, "LOCAL_BOX_INSTALL_FAILED", err.Error(), localBoxStatusDownloading)
	}
	defer os.RemoveAll(workDir)
	if err := writeLocalBoxInstallWorkState(workDir, state); err != nil {
		return m.installFailure(state, "LOCAL_BOX_INSTALL_FAILED", err.Error(), state.Status)
	}
	validated, nextState, err := m.downloadAndVerify(ctx, candidate, workDir, state)
	if err != nil {
		return m.installFailure(state, localBoxErrorCode(err, "LOCAL_BOX_PACKAGE_INVALID"), err.Error(), state.Progress.Phase)
	}
	state = nextState
	state.Status = localBoxStatusInstalling
	state.Progress.Phase = localBoxStatusInstalling
	m.publishState(state)
	if err := os.MkdirAll(m.paths.rootDir, 0o700); err != nil {
		return m.installFailure(state, "LOCAL_BOX_INSTALL_FAILED", err.Error(), localBoxStatusInstalling)
	}
	dataWasEmpty, err := dataDirectoryWasEmpty(m.paths.dataDir)
	if err != nil {
		return m.installFailure(state, "LOCAL_BOX_INSTALL_FAILED", err.Error(), localBoxStatusInstalling)
	}
	committed := false
	defer func() {
		if !committed {
			_ = cleanupNewDataDirectory(m.paths.dataDir, dataWasEmpty)
		}
	}()
	dataIdentity, err := ensureInstallData(m.paths)
	if err != nil {
		return m.installFailure(state, localBoxErrorCodeFrom(err, "LOCAL_BOX_INSTALL_FAILED"), err.Error(), localBoxStatusInstalling)
	}
	installIdentity, err := localrun.NewIdentity(localrun.IdentityKindInstall)
	if err != nil {
		return m.installFailure(state, "LOCAL_BOX_INSTALL_FAILED", err.Error(), localBoxStatusInstalling)
	}
	store, err := m.programStore()
	if err != nil {
		return m.installFailure(state, "LOCAL_BOX_INSTALL_FAILED", err.Error(), localBoxStatusInstalling)
	}
	prepared, err := store.PrepareVersion(ctx, validated.Directory, validated.Product, validated.Files)
	if err != nil {
		return m.installFailure(state, "LOCAL_BOX_INSTALL_FAILED", fmt.Sprintf("准备业务端程序失败：%v", err), localBoxStatusInstalling)
	}
	if err := store.Activate(ctx, prepared, ""); err != nil {
		_ = os.RemoveAll(m.paths.programStoreDir)
		return m.installFailure(state, "LOCAL_BOX_INSTALL_FAILED", fmt.Sprintf("启用业务端程序失败：%v", err), localBoxStatusInstalling)
	}
	record := localBoxInstallRecordFromProduct(validated.Product, m.paths, installIdentity, dataIdentity.DataIdentity, m.sourceKind())
	if err := m.writeInstall(record); err != nil {
		return m.installFailure(state, "LOCAL_BOX_INSTALL_FAILED", err.Error(), localBoxStatusInstalling)
	}
	committed = true
	state.Installed = true
	state.CurrentVersion = prepared.Version
	program := release.CurrentProgram{Version: prepared.Version, ProgramDirectory: prepared.Directory}
	return m.startLocked(ctx, &record, program, state)
}

func (m *localBoxManager) installFailure(state localBoxState, code string, message string, phase string) (localBoxState, error) {
	state = localBoxFailure(state, code, message, phase)
	m.publishState(state)
	return state, nil
}

// recordSourceMismatch 判断已安装记录来源与当前来源类别是否一致。
// 旧记录没有来源字段时按官方来源处理。来源不匹配时不能继续使用已安装程序，
// 只能由用户明确发起重新安装，不能静默替换或继续使用旧来源成品。
func (m *localBoxManager) recordSourceMismatch(record *localBoxInstallRecord) bool {
	if record == nil {
		return false
	}
	return localBoxSourceKind(record.Source).normalize() != m.sourceKind()
}

// removeLegacyProgramLayout 在旧版平铺布局（v1 安装记录）存在时一次性清理：
// 先按 preparePreviousRegistration 的同一规则确认没有仍在运行的旧受托业务端（有则失败不清理），
// 再删除旧平铺 <rootDir>/program 与 install.json，然后按全新安装继续。
// 不做静默转换：旧平铺目录没有 versions/ 结构与逐文件核对事实，转换等于猜测。
func (m *localBoxManager) removeLegacyProgramLayout() error {
	if err := preparePreviousRegistration(m.paths.registrationPath); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(m.paths.rootDir, "program")); err != nil {
		return fmt.Errorf("清理旧版平铺程序目录失败：%w", err)
	}
	if err := os.Remove(m.paths.installPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("清理旧版安装记录失败：%w", err)
	}
	return nil
}

// prepareReinstall 在来源不匹配时清理程序版本仓库和安装记录，保留数据目录。
// 安装记录已经通过严格核对，程序目录属于本次安装事实，可以安全清理。
func (m *localBoxManager) prepareReinstall(record *localBoxInstallRecord) error {
	if err := os.RemoveAll(m.paths.programStoreDir); err != nil {
		return fmt.Errorf("清理旧业务端程序失败：%w", err)
	}
	if err := os.Remove(m.paths.installPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("清理旧安装记录失败：%w", err)
	}
	return nil
}

func (m *localBoxManager) start(ctx context.Context) (localBoxState, error) {
	m.operation.Lock()
	defer m.operation.Unlock()
	if err := m.recoverPendingUpdate(ctx); err != nil {
		state := localBoxFailure(initialLocalBoxState(), localBoxErrorCodeFrom(err, localBoxErrorUpdateFailed), err.Error(), localBoxStatusStarting)
		m.publishState(state)
		return state, nil
	}
	record, err := m.readInstall()
	if err != nil {
		if isLocalBoxLegacyLayout(err) {
			state := localBoxFailure(initialLocalBoxState(), "LOCAL_BOX_LEGACY_LAYOUT", "旧版平铺安装布局，需要重新安装；数据不受影响", localBoxStatusStarting)
			m.publishState(state)
			return state, nil
		}
		state := localBoxFailure(initialLocalBoxState(), "LOCAL_BOX_INSTALL_FAILED", err.Error(), localBoxStatusStarting)
		m.publishState(state)
		return state, nil
	}
	if record == nil {
		return m.readLatestCandidate(ctx, initialLocalBoxState())
	}
	current, err := m.currentProgramFacts()
	if err != nil {
		state := localBoxFailure(initialLocalBoxState(), "LOCAL_BOX_INSTALL_FAILED", "程序资料不完整，需要重新安装", localBoxStatusStarting)
		m.publishState(state)
		return state, nil
	}
	if process := m.currentProcess(); process != nil {
		state := installStateFor(record, current)
		state.Status = localBoxStatusConnected
		state.Connected = true
		m.publishState(state)
		return state, nil
	}
	return m.startLocked(ctx, record, current, installStateFor(record, current))
}

func (m *localBoxManager) startLocked(ctx context.Context, record *localBoxInstallRecord, program release.CurrentProgram, state localBoxState) (localBoxState, error) {
	state.Status = localBoxStatusStarting
	state.Connected = false
	m.publishState(state)
	// 先尝试精准重连后台运行中的同一业务端；任何失败都回退到完整启动流程，
	// 由 preparePreviousRegistration 完成数据占用、旧登记和进程身份的安全判断。
	process, err := reconnectBackgroundBox(ctx, *record, program, m.paths)
	if err == nil {
		m.mu.Lock()
		m.process = process
		m.connection = process.connection
		onConnect := m.onConnect
		m.mu.Unlock()
		if onConnect != nil {
			onConnect(process.connection)
		}
		state.Status = localBoxStatusConnected
		state.Connected = true
		m.publishState(state)
		go m.monitor(process, program.Version)
		return state, nil
	}
	process, err = startLocalBoxProcess(ctx, m.paths, *record, program)
	if err != nil {
		return m.installFailure(state, localBoxErrorCode(err, "LOCAL_BOX_START_FAILED"), err.Error(), localBoxStatusStarting)
	}
	m.mu.Lock()
	m.process = process
	m.connection = process.connection
	onConnect := m.onConnect
	m.mu.Unlock()
	if onConnect != nil {
		onConnect(process.connection)
	}
	state.Status = localBoxStatusConnected
	state.Connected = true
	m.publishState(state)
	go m.monitor(process, program.Version)
	return state, nil
}

func localBoxErrorCode(err error, fallback string) string {
	return localBoxErrorCodeFrom(err, fallback)
}

func (m *localBoxManager) monitor(process *localBoxProcess, version string) {
	if process.reconnected {
		m.monitorReconnected(process, version)
		return
	}
	<-process.done
	m.finishProcess(process, version)
	process.cleanup()
}

// monitorReconnected 轮询后台业务端进程是否真实结束：
// 登记中的进程身份不再匹配时视为业务端已结束，清理客户端连接状态。
func (m *localBoxManager) monitorReconnected(process *localBoxProcess, version string) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-process.done:
			m.finishProcess(process, version)
			return
		case <-ticker.C:
			matches, err := localrun.ProcessMatches(process.registration.ProcessID, process.registration.ProcessStartedAt)
			if err == nil && !matches {
				m.finishProcess(process, version)
				return
			}
		}
	}
}

func (m *localBoxManager) finishProcess(process *localBoxProcess, version string) {
	m.mu.Lock()
	if m.process != process {
		m.mu.Unlock()
		return
	}
	m.process = nil
	m.connection = nil
	onDisconnect := m.onDisconnect
	state := m.state
	state.Installed = true
	state.CurrentVersion = version
	state.Connected = false
	state.Status = localBoxStatusStopped
	m.mu.Unlock()
	if onDisconnect != nil {
		onDisconnect()
	}
	m.publishState(state)
}

func (m *localBoxManager) stop(ctx context.Context) (localBoxState, error) {
	m.operation.Lock()
	defer m.operation.Unlock()
	state := m.currentState()
	process := m.currentProcess()
	if process == nil {
		state.Status = localBoxStatusStopped
		state.Connected = false
		m.clearConnection()
		m.publishState(state)
		return state, nil
	}
	state.Status = localBoxStatusStopping
	state.Connected = false
	m.publishState(state)
	if err := process.requestStop(ctx); err != nil {
		return m.installFailure(state, "LOCAL_BOX_STOP_FAILED", err.Error(), localBoxStatusStopping)
	}
	return m.waitStoppedAndClear(ctx, process, state)
}

// waitStoppedAndClear 等待业务端进程真实结束并清理客户端连接状态；
// 必须在操作互斥锁内调用，供 stop 与 update 共用。
func (m *localBoxManager) waitStoppedAndClear(ctx context.Context, process *localBoxProcess, state localBoxState) (localBoxState, error) {
	if process.reconnected {
		if err := waitBackgroundProcessExit(ctx, process.registration); err != nil {
			return m.installFailure(state, "LOCAL_BOX_NOT_STOPPED", err.Error(), localBoxStatusStopping)
		}
	} else if err := process.wait(ctx); err != nil {
		return m.installFailure(state, "LOCAL_BOX_NOT_STOPPED", err.Error(), localBoxStatusStopping)
	}
	m.mu.Lock()
	if m.process == process {
		m.process = nil
		m.connection = nil
	}
	onDisconnect := m.onDisconnect
	m.mu.Unlock()
	if onDisconnect != nil {
		onDisconnect()
	}
	if !process.reconnected {
		process.cleanup()
	}
	state.Status = localBoxStatusStopped
	state.Installed = true
	m.publishState(state)
	return state, nil
}

// waitBackgroundProcessExit 等待后台业务端进程真实结束；
// 登记中的进程身份不再匹配时视为已经退出。
func waitBackgroundProcessExit(ctx context.Context, registration localrun.Registration) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			matches, err := localrun.ProcessMatches(registration.ProcessID, registration.ProcessStartedAt)
			if err != nil {
				return fmt.Errorf("核对后台业务端进程失败：%w", err)
			}
			if !matches {
				return nil
			}
		}
	}
}

func (m *localBoxManager) currentProcess() *localBoxProcess {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.process
}
