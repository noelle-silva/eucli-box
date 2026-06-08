package fileoperator

import (
	"fmt"
	"os"

	"eucli-box/pkg/types"
)

func runWrite(input types.ToolExecutionInput, config Config, policy PathPolicy) types.ToolExecutionOutput {
	pathArg, err := stringArgument(input, "path", true)
	if err != nil {
		return failure("parse write request", err, nil)
	}
	content, err := rawStringArgument(input, "content", true)
	if err != nil {
		return failure("parse write request", err, nil)
	}
	resolved, err := policy.Resolve(pathArg)
	if err != nil {
		return failure("resolve write path", err, nil)
	}
	if err := ensureParentCreatable(resolved.Absolute); err != nil {
		return failure("resolve write parent", err, baseMetadata("write", resolved))
	}
	metadata := baseMetadata("write", resolved)
	contentBytes := []byte(content)
	metadata["bytes"] = len(contentBytes)
	if err := validateWritableText(contentBytes, config.MaxFileBytes); err != nil {
		return failure("write file", err, metadata)
	}
	existed := false
	providedHash, err := expectedHashArgument(input)
	if err != nil {
		return failure("parse write request", err, metadata)
	}
	if info, err := os.Stat(resolved.Absolute); err == nil {
		if info.IsDir() {
			return failure("write file", fmt.Errorf("path is a directory"), metadata)
		}
		existed = true
		resolvedExisting, err := policy.ResolveExisting(pathArg)
		if err != nil {
			return failure("resolve write path", err, metadata)
		}
		resolved = resolvedExisting
		metadata = baseMetadata("write", resolved)
		metadata["bytes"] = len(contentBytes)
		if _, err := ensureExpectedHash(resolved.Absolute, providedHash, config.MaxFileBytes); err != nil {
			return failure("write file", err, metadata)
		}
	} else if !os.IsNotExist(err) {
		return failure("write file", err, metadata)
	}
	if err := writeTextFile(resolved.Absolute, contentBytes); err != nil {
		return failure("write file", err, metadata)
	}
	metadata["created"] = !existed
	metadata["hash"] = hashBytes(contentBytes)
	verb := "Updated"
	if !existed {
		verb = "Created"
	}
	return success(fmt.Sprintf("%s %s (%d bytes).", verb, resolved.Display, len(contentBytes)), metadata)
}

func expectedHashArgument(input types.ToolExecutionInput) (string, error) {
	value, ok := argumentValue(input, "expectedHash")
	if !ok {
		return "", nil
	}
	return stringValue(value, "expectedHash")
}
