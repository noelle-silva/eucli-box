package everything

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type searchResponse struct {
	Query        string
	Limit        int
	ScopePath    string
	ScopePaths   []string
	ScopeMode    string
	InstanceName string
	DurationMs   int64
	Results      []searchResult
}

func searchEverything(ctx context.Context, executable string, request searchRequest) (searchResponse, error) {
	startedAt := time.Now()
	response := searchResponse{Query: request.Query, Limit: request.MaxResults, ScopePath: request.ScopePath, ScopePaths: request.ScopePaths, ScopeMode: request.ScopeMode, InstanceName: request.InstanceName, Results: []searchResult{}}
	text, err := runEverythingSearchCSV(ctx, time.Duration(request.TimeoutMs)*time.Millisecond, executable, request)
	response.DurationMs = int64(time.Since(startedAt) / time.Millisecond)
	if err != nil {
		return response, err
	}
	results, err := parseSearchCSV(text)
	if err != nil {
		return response, err
	}
	response.Results = results
	return response, nil
}

func runEverythingSearchCSV(ctx context.Context, timeout time.Duration, executable string, request searchRequest) (string, error) {
	if timeout <= 0 {
		return "", fmt.Errorf("search timeout must be positive")
	}
	file, err := os.CreateTemp("", "eucli-everything-search-*.csv")
	if err != nil {
		return "", fmt.Errorf("create Everything search export failed: %w", err)
	}
	csvPath := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(csvPath)
		return "", fmt.Errorf("close Everything search export failed: %w", err)
	}
	defer os.Remove(csvPath)

	args := everythingSearchArgs(request.InstanceName, request.Query, request.MaxResults, csvPath, request.ScopePath, request.ConnectTimeoutMs)
	if _, err := runCommandOutput(ctx, timeout, executable, args...); err != nil {
		return "", err
	}
	content, err := os.ReadFile(csvPath)
	if err != nil {
		return "", fmt.Errorf("read Everything search export failed: %w", err)
	}
	return string(content), nil
}

func everythingSearchArgs(instance string, query string, limit int, csvPath string, scopePath string, connectTimeoutMs int) []string {
	args := []string{}
	if strings.TrimSpace(instance) != "" {
		args = append(args, "-instance", strings.TrimSpace(instance))
	}
	args = append(args,
		"-timeout", strconv.Itoa(connectTimeoutMs),
		"-export-csv", csvPath,
		"-utf8-bom",
		"-no-header",
		"-name",
		"-path-column",
		"-size",
		"-date-modified",
		"-n", strconv.Itoa(limit),
	)
	if strings.TrimSpace(scopePath) != "" {
		args = append(args, "-path", scopePath)
	}
	return append(args, query)
}

func runCommandOutput(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, name, args...)
	output, err := cmd.CombinedOutput()
	text := string(output)
	message := strings.TrimSpace(text)
	if commandCtx.Err() != nil {
		if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
			if message != "" {
				return text, fmt.Errorf("Everything search timed out after %s: %s", timeout, message)
			}
			return text, fmt.Errorf("Everything search timed out after %s", timeout)
		}
		if message != "" {
			return text, fmt.Errorf("Everything search cancelled: %s", message)
		}
		return text, fmt.Errorf("Everything search cancelled")
	}
	if err != nil {
		if message != "" {
			return text, fmt.Errorf("%w: %s", err, message)
		}
		return text, err
	}
	return text, nil
}
