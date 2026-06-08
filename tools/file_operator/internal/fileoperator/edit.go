package fileoperator

import (
	"fmt"
	"os"
	"strings"

	"eucli-box/pkg/types"
)

func runEdit(input types.ToolExecutionInput, config Config, policy PathPolicy) types.ToolExecutionOutput {
	pathArg, err := stringArgument(input, "path", true)
	if err != nil {
		return failure("parse edit request", err, nil)
	}
	oldString, err := rawStringArgument(input, "oldString", true)
	if err != nil {
		return failure("parse edit request", err, nil)
	}
	newString, err := rawStringArgument(input, "newString", true)
	if err != nil {
		return failure("parse edit request", err, nil)
	}
	replaceAll, err := boolArgument(input, "replaceAll", false)
	if err != nil {
		return failure("parse edit request", err, nil)
	}
	resolved, err := policy.Resolve(pathArg)
	if err != nil {
		return failure("resolve edit path", err, nil)
	}
	metadata := baseMetadata("edit", resolved)
	if oldString == newString {
		return failure("edit file", fmt.Errorf("oldString and newString must be different"), metadata)
	}
	providedHash, err := expectedHashArgument(input)
	if err != nil {
		return failure("parse edit request", err, metadata)
	}
	if oldString == "" {
		return createMissingByEdit(config, policy, resolved, newString)
	}
	resolvedExisting, err := policy.ResolveExisting(pathArg)
	if err != nil {
		return failure("resolve edit path", err, metadata)
	}
	metadata = baseMetadata("edit", resolvedExisting)
	data, currentHash, err := readTextFile(resolvedExisting.Absolute, config.MaxFileBytes)
	if err != nil {
		return failure("edit file", err, metadata)
	}
	if expected := strings.TrimSpace(providedHash); expected != "" && expected != currentHash {
		metadata["currentHash"] = currentHash
		return failure("edit file", fmt.Errorf("file changed since expectedHash; current hash is %s", currentHash), metadata)
	}
	content := string(data)
	count := strings.Count(content, oldString)
	if count == 0 {
		return failure("edit file", fmt.Errorf("oldString was not found"), metadata)
	}
	if count > 1 && !replaceAll {
		return failure("edit file", fmt.Errorf("oldString appears %d times; set replaceAll=true or provide a more specific oldString", count), metadata)
	}
	updated := ""
	if replaceAll {
		updated = strings.ReplaceAll(content, oldString, newString)
	} else {
		updated = strings.Replace(content, oldString, newString, 1)
	}
	updatedBytes := []byte(updated)
	if err := validateWritableText(updatedBytes, config.MaxFileBytes); err != nil {
		return failure("edit file", err, metadata)
	}
	if err := writeTextFile(resolvedExisting.Absolute, updatedBytes); err != nil {
		return failure("edit file", err, metadata)
	}
	metadata["hash"] = hashBytes(updatedBytes)
	metadata["previousHash"] = currentHash
	metadata["replacements"] = count
	if !replaceAll {
		metadata["replacements"] = 1
	}
	return success(fmt.Sprintf("Edited %s (%v replacement(s)).", resolvedExisting.Display, metadata["replacements"]), metadata)
}

func createMissingByEdit(config Config, policy PathPolicy, resolved ResolvedPath, content string) types.ToolExecutionOutput {
	metadata := baseMetadata("edit", resolved)
	if _, err := os.Stat(resolved.Absolute); err == nil {
		return failure("edit file", fmt.Errorf("empty oldString can only create a missing file"), metadata)
	} else if !os.IsNotExist(err) {
		return failure("edit file", err, metadata)
	}
	if err := ensureParentCreatable(resolved.Absolute); err != nil {
		return failure("resolve edit parent", err, metadata)
	}
	contentBytes := []byte(content)
	if err := validateWritableText(contentBytes, config.MaxFileBytes); err != nil {
		return failure("edit file", err, metadata)
	}
	if err := writeTextFile(resolved.Absolute, contentBytes); err != nil {
		return failure("edit file", err, metadata)
	}
	metadata["created"] = true
	metadata["hash"] = hashBytes(contentBytes)
	return success(fmt.Sprintf("Created %s via edit.", resolved.Display), metadata)
}
