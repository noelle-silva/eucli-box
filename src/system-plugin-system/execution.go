package systemplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"eucli-box/pkg/types"
)

const pluginPlaceholderAction = "resolve_placeholders"

type persistentProcess struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *json.Decoder
	timeout time.Duration
	mu      sync.Mutex
}

func (s *system) ResolvePlaceholderValues(ctx context.Context) ([]types.SystemPluginPlaceholderValue, []types.PlaceholderProblem) {
	records, err := s.discover(ctx)
	if err != nil {
		return nil, nil
	}
	type recordResult struct {
		index    int
		values   []types.SystemPluginPlaceholderValue
		problems []types.PlaceholderProblem
	}
	results := make(chan recordResult, len(records))
	var wait sync.WaitGroup
	for index, record := range records {
		wait.Add(1)
		go func(index int, record pluginRecord) {
			defer wait.Done()
			values, problems := s.resolveRecordValues(ctx, record)
			results <- recordResult{index: index, values: values, problems: problems}
		}(index, record)
	}
	wait.Wait()
	close(results)
	ordered := make([]recordResult, len(records))
	for result := range results {
		ordered[result.index] = result
	}
	values := []types.SystemPluginPlaceholderValue{}
	problems := []types.PlaceholderProblem{}
	for _, result := range ordered {
		values = append(values, result.values...)
		problems = append(problems, result.problems...)
	}
	return values, problems
}

func (s *system) resolveRecordValues(ctx context.Context, record pluginRecord) ([]types.SystemPluginPlaceholderValue, []types.PlaceholderProblem) {
	if record.status != types.SystemPluginStatusActive {
		return nil, pluginProblems(record)
	}
	if record.manifest.LifecycleType == types.SystemPluginLifecycleCachedHeartbeat {
		resolved, ok := s.cachedValuesForRecord(record)
		if !ok {
			s.setFailure(record.manifest.ID, "cached system plugin value is not ready")
			return nil, pluginProblems(record)
		}
		return resolved, nil
	}
	resolved, err := s.resolveRecord(ctx, record)
	if err != nil {
		s.setFailure(record.manifest.ID, err.Error())
		return nil, pluginProblems(record)
	}
	s.setFailure(record.manifest.ID, "")
	return resolved, nil
}

func (s *system) resolveRecord(ctx context.Context, record pluginRecord) ([]types.SystemPluginPlaceholderValue, error) {
	request, err := s.requestForRecord(record)
	if err != nil {
		return nil, err
	}
	var response types.SystemPluginPlaceholderResponse
	if record.manifest.LifecycleType == types.SystemPluginLifecyclePersistent {
		process, err := s.ensurePersistentProcess(ctx, record)
		if err != nil {
			return nil, err
		}
		response, err = process.request(ctx, request)
		if err != nil {
			s.dropPersistentProcess(record.manifest.ID)
			return nil, err
		}
	} else {
		response, err = s.requestOnDemand(ctx, record, request)
		if err != nil {
			return nil, err
		}
	}
	if response.Status != "success" {
		return nil, pluginExecutionFailed(nonEmpty(response.Error, "system plugin returned failed status"), nil)
	}
	out := make([]types.SystemPluginPlaceholderValue, 0, len(record.manifest.PlaceholderInterfaces))
	for _, item := range record.manifest.PlaceholderInterfaces {
		value, ok := response.Values[item.ID]
		if !ok {
			continue
		}
		out = append(out, types.SystemPluginPlaceholderValue{PluginID: record.manifest.ID, InterfaceID: item.ID, Name: record.effectiveName(item), Value: value})
	}
	return out, nil
}

func (s *system) requestForRecord(record pluginRecord) (types.SystemPluginPlaceholderRequest, error) {
	hostWorkingDirectory, err := os.Getwd()
	if err != nil {
		return types.SystemPluginPlaceholderRequest{}, pluginExecutionFailed("failed to resolve host working directory", err)
	}
	if strings.TrimSpace(record.dataDirectory) == "" {
		return types.SystemPluginPlaceholderRequest{}, pluginExecutionFailed("system plugin data directory is unavailable", nil)
	}
	if err := os.MkdirAll(record.dataDirectory, 0o755); err != nil {
		return types.SystemPluginPlaceholderRequest{}, pluginExecutionFailed("failed to create system plugin data directory", err)
	}
	return types.SystemPluginPlaceholderRequest{
		Action:                pluginPlaceholderAction,
		PluginID:              record.manifest.ID,
		PlaceholderInterfaces: record.interfaceViews(),
		UserConfig:            copyMap(record.userConfig.UserConfig),
		DefaultConfig:         copyMap(record.defaultConfig),
		PluginDirectory:       record.directory,
		PluginDataDirectory:   record.dataDirectory,
		HostWorkingDirectory:  hostWorkingDirectory,
	}, nil
}

func (s *system) requestOnDemand(ctx context.Context, record pluginRecord, request types.SystemPluginPlaceholderRequest) (types.SystemPluginPlaceholderResponse, error) {
	input, err := json.Marshal(request)
	if err != nil {
		return types.SystemPluginPlaceholderResponse{}, pluginExecutionFailed("failed to encode system plugin request", err)
	}
	pluginCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	cmd := exec.CommandContext(pluginCtx, record.executable)
	cmd.Dir = record.directory
	cmd.Stdin = bytes.NewReader(input)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if errors.Is(pluginCtx.Err(), context.DeadlineExceeded) {
		return types.SystemPluginPlaceholderResponse{}, pluginExecutionFailed("system plugin execution timed out", nil)
	}
	if err != nil {
		return types.SystemPluginPlaceholderResponse{}, pluginExecutionFailed(nonEmpty(stderr.String(), err.Error()), err)
	}
	var response types.SystemPluginPlaceholderResponse
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &response); err != nil {
		return types.SystemPluginPlaceholderResponse{}, pluginExecutionFailed("system plugin output is not valid json", err)
	}
	return response, nil
}

func (s *system) ensurePersistentProcess(ctx context.Context, record pluginRecord) (*persistentProcess, error) {
	s.mu.Lock()
	if process := s.persistent[record.manifest.ID]; process != nil {
		s.mu.Unlock()
		return process, nil
	}
	s.mu.Unlock()
	process, err := s.startPersistentProcess(ctx, record)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if existing := s.persistent[record.manifest.ID]; existing != nil {
		s.mu.Unlock()
		process.close(context.Background())
		return existing, nil
	}
	s.persistent[record.manifest.ID] = process
	s.mu.Unlock()
	return process, nil
}

func (s *system) startPersistentProcess(ctx context.Context, record pluginRecord) (*persistentProcess, error) {
	if err := ctx.Err(); err != nil {
		return nil, pluginExecutionFailed("system plugin startup cancelled", err)
	}
	cmd := exec.Command(record.executable)
	cmd.Dir = record.directory
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, pluginExecutionFailed("failed to open system plugin stdin", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, pluginExecutionFailed("failed to open system plugin stdout", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, pluginExecutionFailed(nonEmpty(stderr.String(), "failed to start system plugin"), err)
	}
	return &persistentProcess{cmd: cmd, stdin: stdin, stdout: json.NewDecoder(stdout), timeout: s.timeout}, nil
}

func (p *persistentProcess) request(ctx context.Context, request types.SystemPluginPlaceholderRequest) (types.SystemPluginPlaceholderResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return types.SystemPluginPlaceholderResponse{}, pluginExecutionFailed("system plugin request cancelled", err)
	}
	if err := json.NewEncoder(p.stdin).Encode(request); err != nil {
		return types.SystemPluginPlaceholderResponse{}, pluginExecutionFailed("failed to write system plugin request", err)
	}
	type result struct {
		response types.SystemPluginPlaceholderResponse
		err      error
	}
	done := make(chan result, 1)
	go func() {
		var response types.SystemPluginPlaceholderResponse
		err := p.stdout.Decode(&response)
		done <- result{response: response, err: err}
	}()
	select {
	case <-ctx.Done():
		return types.SystemPluginPlaceholderResponse{}, pluginExecutionFailed("system plugin request cancelled", ctx.Err())
	case item := <-done:
		if item.err != nil {
			return types.SystemPluginPlaceholderResponse{}, pluginExecutionFailed("failed to read system plugin response", item.err)
		}
		return item.response, nil
	case <-time.After(p.timeout):
		return types.SystemPluginPlaceholderResponse{}, pluginExecutionFailed("system plugin request timed out", nil)
	}
}

func (p *persistentProcess) close(ctx context.Context) {
	_ = p.stdin.Close()
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	done := make(chan struct{})
	go func() {
		_ = p.cmd.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
	case <-done:
	}
}

func (s *system) dropPersistentProcess(pluginID string) {
	s.mu.Lock()
	process := s.persistent[pluginID]
	delete(s.persistent, pluginID)
	s.mu.Unlock()
	if process != nil {
		process.close(context.Background())
	}
}

func pluginProblems(record pluginRecord) []types.PlaceholderProblem {
	problems := make([]types.PlaceholderProblem, 0, len(record.manifest.PlaceholderInterfaces))
	for _, item := range record.manifest.PlaceholderInterfaces {
		problems = append(problems, types.PlaceholderProblem{Name: record.effectiveName(item), Type: types.PlaceholderProblemPluginFailed})
	}
	return problems
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
