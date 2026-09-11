package filereader

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
		return listDirectory(input, config, resolved, "read")
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
	decoded, err := decodeText(data)
	if err != nil {
		return failure("decode file text", err, metadata)
	}
	offset, limit, err := effectiveReadWindow(input, config)
	if err != nil {
		return failure("parse read window", err, metadata)
	}
	maxOutput, err := effectiveMaxOutput(input, config)
	if err != nil {
		return failure("parse output limit", err, metadata)
	}
	lines := splitLines(decoded.Text)
	start := offset - 1
	if start > len(lines) {
		start = len(lines)
	}
	end := start + limit
	if end > len(lines) {
		end = len(lines)
	}
	lineTruncated := false
	var builder strings.Builder
	for i := start; i < end; i++ {
		line, truncated := truncateLine(lines[i], config.MaxLineChars)
		if truncated {
			lineTruncated = true
		}
		builder.WriteString(fmt.Sprintf("%d: %s\n", i+1, line))
	}
	payload := builder.String()
	if decoded.InvalidUTF8 {
		payload = fmt.Sprintf("[file_reader warning] File is not valid UTF-8 text; %d byte(s) were replaced with '�'.\n%s", decoded.ReplacementCount, payload)
	}
	windowTruncated := end < len(lines)
	facts := []resultFact{
		textFact("hash", hash),
		intFact("totalLines", len(lines)),
		intFact("returnedLines", end-start),
	}
	if decoded.Encoding != "utf-8" {
		facts = append(facts, textFact("encoding", decoded.Encoding))
	}
	if decoded.InvalidUTF8 {
		facts = append(facts, boolFact("invalidUTF8", true), intFact("utf8ReplacementCount", decoded.ReplacementCount))
	}
	if lineTruncated {
		facts = append(facts, boolFact("lineTruncated", true))
	}
	if windowTruncated {
		facts = append(facts, intFact("nextOffset", end+1))
	}
	facts = append(facts, boolFact("truncated", windowTruncated))
	content, outputTruncated := composeContent(payload, "read", facts, maxOutput)
	metadata["hash"] = hash
	metadata["offset"] = offset
	metadata["limit"] = limit
	metadata["returnedLines"] = end - start
	metadata["totalLines"] = len(lines)
	metadata["truncated"] = outputTruncated || windowTruncated
	if windowTruncated {
		metadata["nextOffset"] = end + 1
	}
	if lineTruncated {
		metadata["lineTruncated"] = true
	}
	if decoded.Encoding != "utf-8" {
		metadata["encoding"] = decoded.Encoding
	}
	if decoded.InvalidUTF8 {
		metadata["invalidUTF8"] = true
		metadata["utf8ReplacementCount"] = decoded.ReplacementCount
	}
	return success(content, metadata)
}
