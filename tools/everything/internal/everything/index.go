package everything

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

// executeIndex implements the full-disk index action: it prepares the everyday
// full-disk instance through the same readiness path the full-disk search
// uses, persists the index database, reports the verifiable facts and never
// runs a search.
func executeIndex(ctx context.Context, input types.ToolExecutionInput, config Config, request searchRequest) types.ToolExecutionOutput {
	metadata := indexRequestMetadata(request)
	if runtime.GOOS != "windows" {
		return failure("execute everything index", errStewardUnsupported, metadata)
	}
	provider, err := resolveBundledProvider(config, input.ToolBodyDirectory)
	if err != nil {
		return failure("resolve bundled Everything provider", err, metadata)
	}
	metadata["provider"] = provider.ID
	metadata["executableSource"] = provider.ExecutableSource
	metadata["runtimeSource"] = provider.RuntimeSource
	if _, err := requireHealthySteward(ctx, config, provider.RuntimeExecutable, metadata); err != nil {
		return failure("check Everything permission steward", err, metadata)
	}
	startedAt := time.Now()
	lock, err := acquireBundledRuntimeLock(ctx, input.ToolDataDirectory, config)
	if err != nil {
		return failure("lock bundled Everything runtime", err, metadata)
	}
	defer lock.Release()
	request, err = ensureBundledRuntime(ctx, input.ToolDataDirectory, config, provider, request)
	if err != nil {
		return failure("prepare bundled Everything runtime", err, metadata)
	}
	metadata["instanceName"] = request.InstanceName
	defer retireBundledRuntimeSilently(input.ToolDataDirectory, config, provider, request, input, metadata)
	counts, total, err := countIndexedDrives(ctx, provider.ESExecutable, request)
	metadata["durationMs"] = int64(time.Since(startedAt) / time.Millisecond)
	metadata["driveEntryCounts"] = counts
	metadata["entryCount"] = total
	if err != nil {
		return failure("count Everything indexed entries", err, metadata)
	}
	return types.ToolExecutionOutput{Status: types.ToolStatusSuccess, Content: indexContent(request, total), Metadata: metadata}
}

func indexRequestMetadata(request searchRequest) map[string]any {
	metadata := map[string]any{
		"action":       string(actionIndex),
		"scopeMode":    request.ScopeMode,
		"scopePaths":   request.ScopePaths,
		"instanceName": request.InstanceName,
		"timeoutMs":    request.TimeoutMs,
	}
	if strings.TrimSpace(request.Description) != "" {
		metadata["description"] = request.Description
	}
	return metadata
}

// countIndexedDrives probes the visible entry count of every drive root once
// the index became ready, so the action reports checkable facts.
func countIndexedDrives(ctx context.Context, executable string, request searchRequest) (map[string]int, int, error) {
	timeout := boundedProbeTimeout(ctx, request.ConnectTimeoutMs)
	counts := make(map[string]int, len(request.ScopePaths))
	total := 0
	for _, drivePath := range request.ScopePaths {
		output, err := runCommandOutput(ctx, timeout, executable, everythingCountArgs(request.InstanceName, int(timeout/time.Millisecond), drivePath)...)
		if err != nil {
			return counts, total, fmt.Errorf("count indexed entries in %s: %w", drivePath, err)
		}
		count, err := parseResultCount(output)
		if err != nil {
			return counts, total, fmt.Errorf("count indexed entries in %s: %w", drivePath, err)
		}
		counts[drivePath] = count
		total += count
	}
	return counts, total, nil
}

// boundedProbeTimeout caps one probe with the caller deadline when one is set,
// so counting can never outlive the caller-specified time.
func boundedProbeTimeout(ctx context.Context, connectTimeoutMs int) time.Duration {
	timeout := time.Duration(connectTimeoutMs) * time.Millisecond
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	return timeout
}

func indexContent(request searchRequest, total int) string {
	var builder strings.Builder
	builder.WriteString("## Everything Full-Disk Index Ready\n\n")
	builder.WriteString(fmt.Sprintf("Scope: all local drives: %s  \n", strings.Join(inlineCodeList(request.ScopePaths), ", ")))
	builder.WriteString(fmt.Sprintf("Instance: `%s`  \n", request.InstanceName))
	builder.WriteString(fmt.Sprintf("Entries: `%d`\n", total))
	return builder.String()
}
