package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"eucli-box/pkg/localrun"
)

type localBoxProcess struct {
	command      *exec.Cmd
	done         chan struct{}
	waitMu       sync.RWMutex
	waitErr      error
	connection   *boxConnection
	registration localrun.Registration
	tempDir      string
	reconnected  bool
}

type localBoxReady struct {
	Type            string `json:"type"`
	Endpoint        string `json:"endpoint"`
	InstallIdentity string `json:"installIdentity"`
	DataIdentity    string `json:"dataIdentity"`
	RunIdentity     string `json:"runIdentity"`
	Version         string `json:"version"`
}

type localBoxRunResponse struct {
	InstallIdentity  string    `json:"installIdentity"`
	DataIdentity     string    `json:"dataIdentity"`
	RunIdentity      string    `json:"runIdentity"`
	ProcessID        int       `json:"processId"`
	ProcessStartedAt time.Time `json:"processStartedAt"`
	Version          string    `json:"version"`
}

func startLocalBoxProcess(ctx context.Context, paths localBoxPaths, record localBoxInstallRecord) (*localBoxProcess, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := os.MkdirAll(paths.runtimeDir, 0o700); err != nil {
		return nil, fmt.Errorf("建立运行目录失败：%w", err)
	}
	if err := preparePreviousRegistration(paths.registrationPath); err != nil {
		return nil, err
	}
	runIdentity, err := localrun.NewIdentity(localrun.IdentityKindRun)
	if err != nil {
		return nil, err
	}
	sessionCredential, err := localrun.NewIdentity(localrun.IdentityKindSession)
	if err != nil {
		return nil, err
	}
	tempDir := filepath.Join(paths.workDir, "runtime-"+runIdentity, "temp")
	if err := os.MkdirAll(tempDir, 0o700); err != nil {
		return nil, fmt.Errorf("建立受托临时目录失败：%w", err)
	}
	command := exec.Command(filepath.Join(record.ProgramDir, "eucli-box.exe"))
	command.Dir = record.ProgramDir
	command.Env = localBoxEnvironment(os.Environ(), map[string]string{
		"EUCLI_BOX_LOCAL_RUN":          "1",
		"EUCLI_BOX_INSTALL_ID":         record.InstallIdentity,
		"EUCLI_BOX_DATA_ID":            record.DataIdentity,
		"EUCLI_BOX_RUN_ID":             runIdentity,
		"EUCLI_BOX_SESSION_CREDENTIAL": sessionCredential,
		"EUCLI_BOX_DATA_DIR":           record.DataDir,
		"EUCLI_BOX_PROGRAM_ROOT":       filepath.Dir(record.ProgramDir),
		"EUCLI_BOX_REGISTRATION_PATH":  paths.registrationPath,
		"EUCLI_BOX_ADDR":               "127.0.0.1:0",
		"TEMP":                         tempDir,
		"TMP":                          tempDir,
	})
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("准备业务端 ready 输出失败：%w", err)
	}
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("启动业务端进程失败：%w", err)
	}
	process := &localBoxProcess{
		command: command,
		done:    make(chan struct{}),
		tempDir: tempDir,
	}
	go func() {
		err := command.Wait()
		process.waitMu.Lock()
		process.waitErr = err
		process.waitMu.Unlock()
		close(process.done)
	}()
	processStartedAt, err := localrun.ProcessStartedAt(command.Process.Pid)
	if err != nil {
		process.terminateAndWait()
		return nil, fmt.Errorf("读取业务端进程开始事实失败：%w", err)
	}
	readyLine := make(chan struct {
		value string
		err   error
	}, 1)
	go func() {
		line, readErr := bufio.NewReader(stdout).ReadString('\n')
		readyLine <- struct {
			value string
			err   error
		}{value: strings.TrimSpace(line), err: readErr}
	}()
	select {
	case <-ctx.Done():
		process.terminateAndWait()
		return nil, fmt.Errorf("读取业务端 ready 被取消：%w", ctx.Err())
	case <-process.done:
		return nil, fmt.Errorf("业务端在 ready 前退出：%w", process.exitError())
	case result := <-readyLine:
		if result.err != nil && result.value == "" {
			process.terminateAndWait()
			return nil, fmt.Errorf("读取业务端 ready 失败：%w", result.err)
		}
		var ready localBoxReady
		if err := json.Unmarshal([]byte(result.value), &ready); err != nil {
			process.terminateAndWait()
			return nil, fmt.Errorf("解析业务端 ready 失败：%w", err)
		}
		if ready.Type != "local-box-ready" || ready.InstallIdentity != record.InstallIdentity || ready.DataIdentity != record.DataIdentity || ready.RunIdentity != runIdentity || ready.Version != record.Version {
			process.terminateAndWait()
			return nil, errors.New("LOCAL_BOX_START_FAILED: ready 运行事实不一致")
		}
		if err := validateLocalEndpoint(ready.Endpoint); err != nil {
			process.terminateAndWait()
			return nil, err
		}
		go io.Copy(io.Discard, stdout)
		registration, err := localrun.ReadRegistration(paths.registrationPath)
		if err != nil {
			process.terminateAndWait()
			return nil, fmt.Errorf("LOCAL_BOX_REGISTRATION_INVALID: %w", err)
		}
		facts := localrun.RegistrationFacts{
			InstallIdentity: record.InstallIdentity, DataIdentity: record.DataIdentity, RunIdentity: runIdentity,
			Endpoint: ready.Endpoint, SessionCredential: sessionCredential, ProcessID: command.Process.Pid,
			ProcessStartedAt: processStartedAt, BoxVersion: record.Version,
		}
		if err := localrun.MatchRegistration(registration, facts); err != nil {
			process.terminateAndWait()
			return nil, fmt.Errorf("LOCAL_BOX_REGISTRATION_INVALID: %w", err)
		}
		connection := &boxConnection{Source: boxConnectionSourceLocal, BaseURL: ready.Endpoint, Credential: sessionCredential}
		if err := verifyLocalRunFacts(ctx, connection, facts); err != nil {
			process.terminateAndWait()
			return nil, err
		}
		process.connection = connection
		process.registration = registration
		return process, nil
	}
}

func preparePreviousRegistration(path string) error {
	registration, err := localrun.ReadRegistration(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("LOCAL_BOX_REGISTRATION_INVALID: %w", err)
	}
	if registration.Status == localrun.RegistrationStatusRunning {
		matches, matchErr := localrun.ProcessMatches(registration.ProcessID, registration.ProcessStartedAt)
		if matchErr != nil {
			return fmt.Errorf("LOCAL_BOX_REGISTRATION_INVALID: 无法核对旧业务端进程：%w", matchErr)
		}
		if matches {
			return errors.New("LOCAL_BOX_DATA_IN_USE: 已有受托业务端仍在运行")
		}
		if markErr := localrun.MarkRegistrationStale(path); markErr != nil {
			return fmt.Errorf("LOCAL_BOX_REGISTRATION_INVALID: 无法标记旧登记：%w", markErr)
		}
	}
	if err := localrun.DeleteRegistration(path); err != nil {
		return fmt.Errorf("LOCAL_BOX_REGISTRATION_INVALID: 无法清理旧登记：%w", err)
	}
	return nil
}

// reconnectBackgroundBox 尝试精准重连后台运行中的同一业务端：
// 只接受登记中安装身份、数据身份、运行身份全部与本次安装记录一致的运行；
// 通过 /api/local-run 真实核对后才复用连接，不猜测端口或进程。
func reconnectBackgroundBox(ctx context.Context, record localBoxInstallRecord, paths localBoxPaths) (*localBoxProcess, error) {
	registration, err := localrun.ReadRegistration(paths.registrationPath)
	if err != nil {
		return nil, err
	}
	if registration.Status != localrun.RegistrationStatusRunning {
		return nil, errors.New("LOCAL_BOX_REGISTRATION_INVALID: 登记不是运行状态")
	}
	if registration.InstallIdentity != record.InstallIdentity || registration.DataIdentity != record.DataIdentity {
		return nil, errors.New("LOCAL_BOX_REGISTRATION_INVALID: 登记安装身份不一致")
	}
	matches, err := localrun.ProcessMatches(registration.ProcessID, registration.ProcessStartedAt)
	if err != nil {
		return nil, fmt.Errorf("LOCAL_BOX_REGISTRATION_INVALID: 无法核对后台业务端进程：%w", err)
	}
	if !matches {
		return nil, errors.New("LOCAL_BOX_REGISTRATION_INVALID: 后台业务端进程已不存在")
	}
	if err := validateLocalEndpoint(registration.Endpoint); err != nil {
		return nil, errors.New("LOCAL_BOX_REGISTRATION_INVALID: 登记地址无效")
	}
	connection := &boxConnection{Source: boxConnectionSourceLocal, BaseURL: registration.Endpoint, Credential: registration.SessionCredential}
	facts := localrun.RegistrationFacts{
		InstallIdentity: record.InstallIdentity, DataIdentity: record.DataIdentity, RunIdentity: registration.RunIdentity,
		Endpoint: registration.Endpoint, SessionCredential: registration.SessionCredential, ProcessID: registration.ProcessID,
		ProcessStartedAt: registration.ProcessStartedAt, BoxVersion: registration.BoxVersion,
	}
	if err := verifyLocalRunFacts(ctx, connection, facts); err != nil {
		return nil, err
	}
	process := &localBoxProcess{
		done:         make(chan struct{}),
		connection:   connection,
		registration: registration,
		reconnected:  true,
	}
	return process, nil
}

func (process *localBoxProcess) requestStop(ctx context.Context) error {	payload, _ := json.Marshal(map[string]string{"runIdentity": process.registration.RunIdentity})
	target, err := url.Parse(strings.TrimRight(process.connection.BaseURL, "/") + "/api/local-run/stop")
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+process.connection.Credential)
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return fmt.Errorf("请求业务端停止失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("请求业务端停止失败：远端返回 %d", response.StatusCode)
	}
	return nil
}

func (process *localBoxProcess) wait(ctx context.Context) error {
	select {
	case <-process.done:
		return process.exitError()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (process *localBoxProcess) terminateAndWait() {
	if process.command.Process != nil {
		_ = process.command.Process.Kill()
	}
	<-process.done
}

func (process *localBoxProcess) exitError() error {
	process.waitMu.RLock()
	defer process.waitMu.RUnlock()
	return process.waitErr
}

func (process *localBoxProcess) cleanup() {
	_ = os.RemoveAll(filepath.Dir(process.tempDir))
}

func verifyLocalRunFacts(ctx context.Context, connection *boxConnection, facts localrun.RegistrationFacts) error {
	target, err := url.Parse(strings.TrimRight(connection.BaseURL, "/") + "/api/local-run")
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+connection.Credential)
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return fmt.Errorf("LOCAL_BOX_REGISTRATION_INVALID: 运行事实读取失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("LOCAL_BOX_REGISTRATION_INVALID: 运行事实返回 %d", response.StatusCode)
	}
	var envelope struct {
		Data localBoxRunResponse `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("LOCAL_BOX_REGISTRATION_INVALID: 运行事实资料无效：%w", err)
	}
	if envelope.Data.InstallIdentity != facts.InstallIdentity || envelope.Data.DataIdentity != facts.DataIdentity || envelope.Data.RunIdentity != facts.RunIdentity || envelope.Data.ProcessID != facts.ProcessID || !envelope.Data.ProcessStartedAt.Equal(facts.ProcessStartedAt) || envelope.Data.Version != facts.BoxVersion {
		return errors.New("LOCAL_BOX_REGISTRATION_INVALID: 业务端返回的运行事实不一致")
	}
	return nil
}

func localBoxEnvironment(base []string, replacements map[string]string) []string {
	result := make([]string, 0, len(base)+len(replacements))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if ok {
			if _, replaced := replacements[strings.ToUpper(key)]; replaced {
				continue
			}
		}
		result = append(result, item)
	}
	for key, value := range replacements {
		result = append(result, key+"="+value)
	}
	return result
}

func validateLocalEndpoint(value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("LOCAL_BOX_START_FAILED: ready 地址不是本机回环地址")
	}
	host, port, err := net.SplitHostPort(parsed.Host)
	portNumber, portErr := strconv.Atoi(port)
	if err != nil || host != "127.0.0.1" || portErr != nil || portNumber < 1 || portNumber > 65535 {
		return errors.New("LOCAL_BOX_START_FAILED: ready 地址不是本机回环地址")
	}
	return nil
}
