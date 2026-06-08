package fileoperator

import (
	"fmt"
	"os"
	"strings"

	"eucli-box/pkg/types"
)

func runRead(input types.ToolExecutionInput, config Config, policy PathPolicy) types.ToolExecutionOutput {
	pathArg, err := stringArgument(input, "path", true)
	if err != nil {
		return failure("parse read request", err, nil)
	}
	resolved, err := policy.ResolveExisting(pathArg)
	if err != nil {
		return failure("resolve read path", err, nil)
	}
	info, err := os.Stat(resolved.Absolute)
	if err != nil {
		return failure("stat read path", err, baseMetadata("read", resolved))
	}
	if info.IsDir() {
		return readDirectory(input, config, resolved)
	}
	return readText(input, config, resolved, info.Size())
}

func readText(input types.ToolExecutionInput, config Config, resolved ResolvedPath, size int64) types.ToolExecutionOutput {
	metadata := baseMetadata("read", resolved)
	metadata["type"] = "file"
	metadata["sizeBytes"] = size
	data, hash, err := readTextFile(resolved.Absolute, config.MaxFileBytes)
	if err != nil {
		return failure("read file", err, metadata)
	}
	offset, limit, err := effectiveReadWindow(input, config)
	if err != nil {
		return failure("parse read window", err, metadata)
	}
	maxOutput, err := effectiveMaxOutput(input, config)
	if err != nil {
		return failure("parse output limit", err, metadata)
	}
	lines := splitLines(string(data))
	start := offset - 1
	if start > len(lines) {
		start = len(lines)
	}
	end := start + limit
	if end > len(lines) {
		end = len(lines)
	}
	var builder strings.Builder
	for i := start; i < end; i++ {
		line, lineTruncated := truncateLine(lines[i], config.MaxLineChars)
		if lineTruncated {
			metadata["lineTruncated"] = true
		}
		builder.WriteString(fmt.Sprintf("%d: %s\n", i+1, line))
	}
	content, outputTruncated := truncateText(builder.String(), maxOutput)
	metadata["hash"] = hash
	metadata["offset"] = offset
	metadata["limit"] = limit
	metadata["returnedLines"] = end - start
	metadata["totalLines"] = len(lines)
	metadata["truncated"] = outputTruncated || end < len(lines)
	if end < len(lines) {
		metadata["nextOffset"] = end + 1
	}
	return success(content, metadata)
}

func readDirectory(input types.ToolExecutionInput, config Config, resolved ResolvedPath) types.ToolExecutionOutput {
	return listDirectory(input, config, resolved)
}
