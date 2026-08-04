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
	programDir       string
	executablePath   string
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
		programDir:       filepath.Join(root, "program"),
		executablePath:   filepath.Join(root, "program", "eucli-box.exe"),
		dataDir:          filepath.Join(root, "data"),
		runtimeDir:       runtimeDir,
		registrationPath: filepath.Join(runtimeDir, "registration.json"),
		workDir:          filepath.Join(root, "work"),
	}, nil
}

type localBoxInstallRecord struct {
	SchemaVersion   int                           `json:"schemaVersion"`
	Artifact        types.ReleaseArtifactIdentity `json:"artifact"`
	Source          string                        `json:"source,omitempty"`
	Version         string                        `json:"version"`
	Platform        string                        `json:"platform"`
	InstallIdentity string                        `json:"installIdentity"`
	DataIdentity    string                        `json:"dataIdentity"`
	ProgramDir      string                        `json:"programDir"`
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
		return nil, localBoxInstallRecordError{err: fmt.Errorf("LOCAL_BOX_INSTALL_FAILED: 安装资料无效：%w", err)}
	}
	return &record, nil
}

func validateLocalBoxInstallRecord(record localBoxInstallRecord, paths localBoxPaths) error {
	if record.SchemaVersion != 1 || record.Artifact.Kind != localBoxArtifactID || record.Artifact.ID != localBoxArtifactID {
		return errors.New("安装资料身份无效")
	}
	if record.Source != "" && !localBoxSourceKind(record.Source).valid() {
		return errors.New("安装资料来源无效")
	}
	if err := release.ValidateVersion(record.Version); err != nil {
		return err
	}
	if record.Platform != types.ReleasePlatformWindowsX64 {
		return errors.New("安装资料平台无效")
	}
	if !filepath.IsAbs(record.ProgramDir) || !filepath.IsAbs(record.DataDir) || !filepath.IsAbs(record.RuntimeDir) {
		return errors.New("安装资料目录必须使用绝对路径")
	}
	if err := localrun.ValidateIdentity(record.InstallIdentity, localrun.IdentityKindInstall); err != nil {
		return err
	}
	if err := localrun.ValidateIdentity(record.DataIdentity, localrun.IdentityKindData); err != nil {
		return err
	}
	for _, pair := range [][2]string{{record.ProgramDir, paths.programDir}, {record.DataDir, paths.dataDir}, {record.RuntimeDir, paths.runtimeDir}} {
		actual, err := filepath.Abs(strings.TrimSpace(pair[0]))
		if err != nil || filepath.Clean(actual) != filepath.Clean(pair[1]) {
			return errors.New("安装资料目录事实无效")
		}
	}
	programInfo, err := os.Stat(filepath.Join(record.ProgramDir, "eucli-box.exe"))
	if err != nil || programInfo.IsDir() {
		return errors.New("安装资料程序文件不存在")
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

func (m *localBoxManager) recoverCorruptInstallRecord() error {
	info, err := os.Stat(m.paths.programDir)
	if errors.Is(err, os.ErrNotExist) {
		return os.Remove(m.paths.installPath)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("损坏安装资料对应的程序目录不是目录")
	}
	entries, err := os.ReadDir(m.paths.programDir)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("损坏安装资料对应的程序目录包含未知内容，拒绝自动覆盖")
	}
	if err := os.Remove(m.paths.programDir); err != nil {
		return err
	}
	return os.Remove(m.paths.installPath)
}

func installStateFor(record *localBoxInstallRecord) localBoxState {
	state := initialLocalBoxState()
	if record == nil {
		return state
	}
	state.Installed = true
	state.CurrentVersion = record.Version
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
	record, err := m.readInstall()
	if err != nil {
		state := localBoxFailure(initialLocalBoxState(), "LOCAL_BOX_INSTALL_FAILED", err.Error(), localBoxStatusNotInstalled)
		m.publishState(state)
		return state, nil
	}
	state := installStateFor(record)
	if record == nil {
		return m.readLatestCandidate(ctx, state)
	}
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
	if m.currentProcess() != nil {
		return m.currentState(), newError("LOCAL_BOX_INSTALL_FAILED", "业务端正在运行，不能重复安装")
	}
	if record, err := m.readInstall(); err != nil {
		if isLocalBoxInstallRecordInvalid(err) {
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
			state := installStateFor(record)
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
	state.LatestVersion = candidate.Manifest.Version
	state.ReleaseNotes = candidate.ReleaseNotes
	state.DownloadSize = candidate.Manifest.Archive.Size
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
	extractedDir := filepath.Join(workDir, "extracted")
	manifest, nextState, err := m.downloadAndVerify(ctx, candidate, workDir, state)
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
	if err := prepareProgramTarget(m.paths.programDir); err != nil {
		return m.installFailure(state, "LOCAL_BOX_INSTALL_FAILED", err.Error(), localBoxStatusInstalling)
	}
	if err := os.Rename(extractedDir, m.paths.programDir); err != nil {
		return m.installFailure(state, "LOCAL_BOX_INSTALL_FAILED", fmt.Sprintf("启用业务端程序失败：%v", err), localBoxStatusInstalling)
	}
	programInstalled := true
	record := localBoxInstallRecordFromManifest(manifest, m.paths, installIdentity, dataIdentity.DataIdentity, m.sourceKind())
	if err := m.writeInstall(record); err != nil {
		if programInstalled {
			_ = os.RemoveAll(m.paths.programDir)
		}
		return m.installFailure(state, "LOCAL_BOX_INSTALL_FAILED", err.Error(), localBoxStatusInstalling)
	}
	committed = true
	state.Installed = true
	state.CurrentVersion = record.Version
	return m.startLocked(ctx, &record, state)
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

// prepareReinstall 在来源不匹配时清理旧程序目录和安装记录，保留数据目录。
// 安装记录已经通过严格核对，程序目录属于本次安装事实，可以安全清理。
func (m *localBoxManager) prepareReinstall(record *localBoxInstallRecord) error {
	if err := os.RemoveAll(record.ProgramDir); err != nil {
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
	record, err := m.readInstall()
	if err != nil {
		state := localBoxFailure(initialLocalBoxState(), "LOCAL_BOX_INSTALL_FAILED", err.Error(), localBoxStatusStarting)
		m.publishState(state)
		return state, nil
	}
	if record == nil {
		return m.readLatestCandidate(ctx, initialLocalBoxState())
	}
	if process := m.currentProcess(); process != nil {
		state := installStateFor(record)
		state.Status = localBoxStatusConnected
		state.Connected = true
		m.publishState(state)
		return state, nil
	}
	return m.startLocked(ctx, record, installStateFor(record))
}

func (m *localBoxManager) startLocked(ctx context.Context, record *localBoxInstallRecord, state localBoxState) (localBoxState, error) {
	state.Status = localBoxStatusStarting
	state.Connected = false
	m.publishState(state)
	process, err := startLocalBoxProcess(ctx, m.paths, *record)
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
	go m.monitor(process, record.Version)
	return state, nil
}

func localBoxErrorCode(err error, fallback string) string {
	return localBoxErrorCodeFrom(err, fallback)
}

func (m *localBoxManager) monitor(process *localBoxProcess, version string) {
	<-process.done
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
	process.cleanup()
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
	if err := process.wait(ctx); err != nil {
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
	process.cleanup()
	state.Status = localBoxStatusStopped
	state.Installed = true
	m.publishState(state)
	return state, nil
}

func (m *localBoxManager) currentProcess() *localBoxProcess {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.process
}

func prepareProgramTarget(path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("程序目录目标不是目录")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("程序目录已经存在内容，拒绝覆盖")
	}
	return os.Remove(path)
}
