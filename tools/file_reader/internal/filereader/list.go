package filereader

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

func runList(input types.ToolExecutionInput, config Config, policy PathPolicy) types.ToolExecutionOutput {
	pathArg, err := stringArgument(input, "path", false)
	if err != nil {
		return failure("parse list request", err, nil)
	}
	resolved, err := policy.ResolveExisting(pathArg)
	if err != nil {
		return failure("resolve list path", err, nil)
	}
	return listDirectory(input, config, resolved, "list")
}

func listDirectory(input types.ToolExecutionInput, config Config, resolved ResolvedPath, action string) types.ToolExecutionOutput {
	metadata := baseMetadata(action, resolved)
	metadata["type"] = "directory"
	entries, err := os.ReadDir(resolved.Absolute)
	if err != nil {
		return failure("list directory", err, metadata)
	}
	showHidden, err := boolArgument(input, "showHidden", false)
	if err != nil {
		return failure("parse list request", err, metadata)
	}
	filtered := entries[:0]
	for _, entry := range entries {
		if !showHidden && isHiddenName(entry.Name()) {
			continue
		}
		filtered = append(filtered, entry)
	}
	sort.Slice(filtered, func(i, j int) bool {
		leftDir := filtered[i].IsDir()
		rightDir := filtered[j].IsDir()
		if leftDir != rightDir {
			return leftDir
		}
		return strings.ToLower(filtered[i].Name()) < strings.ToLower(filtered[j].Name())
	})
	offset, limit, err := effectiveReadWindow(input, config)
	if err != nil {
		return failure("parse list window", err, metadata)
	}
	maxOutput, err := effectiveMaxOutput(input, config)
	if err != nil {
		return failure("parse output limit", err, metadata)
	}
	start := offset - 1
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	var builder strings.Builder
	for i := start; i < end; i++ {
		entry := filtered[i]
		name := entry.Name()
		entryType := "file"
		if entry.IsDir() {
			entryType = "directory"
			name += "/"
		}
		size := int64(0)
		modified := ""
		if info, err := entry.Info(); err == nil {
			size = info.Size()
			modified = info.ModTime().Format(time.RFC3339)
		}
		builder.WriteString(fmt.Sprintf("%d: %s\t%s\t%d\t%s\n", i+1, name, entryType, size, modified))
	}
	windowTruncated := end < len(filtered)
	facts := []resultFact{
		intFact("totalEntries", len(filtered)),
		intFact("returnedEntries", end-start),
	}
	if windowTruncated {
		facts = append(facts, intFact("nextOffset", end+1))
	}
	facts = append(facts, boolFact("truncated", windowTruncated))
	content, outputTruncated := composeContent(builder.String(), action, facts, maxOutput)
	metadata["offset"] = offset
	metadata["limit"] = limit
	metadata["returnedEntries"] = end - start
	metadata["totalEntries"] = len(filtered)
	metadata["truncated"] = outputTruncated || windowTruncated
	if windowTruncated {
		metadata["nextOffset"] = end + 1
	}
	return success(content, metadata)
}
