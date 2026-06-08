package fileoperator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/pkg/types"
)

type patchOperation struct {
	Kind   string
	Path   string
	MoveTo string
	Lines  []string
}

type patchChange struct {
	Path          ResolvedPath
	FinalPath     string
	FinalContent  []byte
	FinalExists   bool
	Original      patchSnapshot
	FinalSnapshot patchSnapshot
}

type patchSnapshot struct {
	Exists  bool
	Content []byte
}

func runApplyPatch(input types.ToolExecutionInput, config Config, policy PathPolicy) types.ToolExecutionOutput {
	patchText, err := rawStringArgument(input, "patchText", true)
	if err != nil {
		return failure("parse apply_patch request", err, nil)
	}
	operations, err := parsePatchText(patchText)
	if err != nil {
		return failure("parse patch", err, map[string]any{"action": "apply_patch"})
	}
	if len(operations) == 0 {
		return failure("parse patch", fmt.Errorf("patch has no file operations"), map[string]any{"action": "apply_patch"})
	}
	metadata := map[string]any{"action": "apply_patch", "operationCount": len(operations)}
	changes, err := planPatchChanges(operations, config, policy)
	if err != nil {
		return failure("plan patch", err, metadata)
	}
	changedPaths, err := commitPatchChanges(changes)
	if err != nil {
		return failure("apply patch", err, metadata)
	}
	metadata["changedPaths"] = changedPaths
	return success(fmt.Sprintf("Applied patch to %d operation(s).", len(operations)), metadata)
}

func parsePatchText(patchText string) ([]patchOperation, error) {
	lines := splitPatchLines(patchText)
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "*** Begin Patch" {
		return nil, fmt.Errorf("patch must start with *** Begin Patch")
	}
	operations := []patchOperation{}
	var current *patchOperation
	for index := 1; index < len(lines); index++ {
		line := lines[index]
		trimmed := strings.TrimSpace(line)
		if trimmed == "*** End Patch" {
			if current != nil {
				operations = append(operations, *current)
			}
			return operations, nil
		}
		if strings.HasPrefix(line, "*** Add File: ") || strings.HasPrefix(line, "*** Delete File: ") || strings.HasPrefix(line, "*** Update File: ") {
			if current != nil {
				operations = append(operations, *current)
			}
			operation, err := parsePatchHeader(line)
			if err != nil {
				return nil, err
			}
			current = &operation
			continue
		}
		if current == nil {
			if trimmed == "" {
				continue
			}
			return nil, fmt.Errorf("patch content appeared before a file header at line %d", index+1)
		}
		if strings.HasPrefix(line, "*** Move to: ") {
			current.MoveTo = strings.TrimSpace(strings.TrimPrefix(line, "*** Move to: "))
			continue
		}
		current.Lines = append(current.Lines, line)
	}
	return nil, fmt.Errorf("patch must end with *** End Patch")
}

func splitPatchLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Split(text, "\n")
}

func parsePatchHeader(line string) (patchOperation, error) {
	switch {
	case strings.HasPrefix(line, "*** Add File: "):
		path := strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: "))
		if path == "" {
			return patchOperation{}, fmt.Errorf("add file path is required")
		}
		return patchOperation{Kind: "add", Path: path}, nil
	case strings.HasPrefix(line, "*** Delete File: "):
		path := strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File: "))
		if path == "" {
			return patchOperation{}, fmt.Errorf("delete file path is required")
		}
		return patchOperation{Kind: "delete", Path: path}, nil
	case strings.HasPrefix(line, "*** Update File: "):
		path := strings.TrimSpace(strings.TrimPrefix(line, "*** Update File: "))
		if path == "" {
			return patchOperation{}, fmt.Errorf("update file path is required")
		}
		return patchOperation{Kind: "update", Path: path}, nil
	default:
		return patchOperation{}, fmt.Errorf("invalid patch file header")
	}
}

func planPatchChanges(operations []patchOperation, config Config, policy PathPolicy) ([]patchChange, error) {
	changes := make([]patchChange, 0, len(operations)*2)
	seenFinals := map[string]struct{}{}
	for _, operation := range operations {
		operationChanges, err := planPatchOperation(operation, config, policy)
		if err != nil {
			return nil, err
		}
		for _, change := range operationChanges {
			key := filepath.Clean(change.FinalPath)
			if _, ok := seenFinals[key]; ok {
				return nil, fmt.Errorf("patch changes the same path more than once: %s", change.Path.Display)
			}
			seenFinals[key] = struct{}{}
			changes = append(changes, change)
		}
	}
	return changes, nil
}

func planPatchOperation(operation patchOperation, config Config, policy PathPolicy) ([]patchChange, error) {
	switch operation.Kind {
	case "add":
		return planAdd(operation, config, policy)
	case "delete":
		return planDelete(operation, config, policy)
	case "update":
		if operation.MoveTo != "" {
			return planMoveUpdate(operation, config, policy)
		}
		return planUpdate(operation, config, policy)
	default:
		return nil, fmt.Errorf("unsupported patch operation %q", operation.Kind)
	}
}

func planAdd(operation patchOperation, config Config, policy PathPolicy) ([]patchChange, error) {
	resolved, err := policy.Resolve(operation.Path)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(resolved.Absolute); err == nil {
		return nil, fmt.Errorf("add file already exists: %s", resolved.Display)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := ensureParentCreatable(resolved.Absolute); err != nil {
		return nil, err
	}
	content, err := addedPatchContent(operation.Lines)
	if err != nil {
		return nil, err
	}
	contentBytes := []byte(content)
	if err := validateWritableText(contentBytes, config.MaxFileBytes); err != nil {
		return nil, err
	}
	return []patchChange{{Path: resolved, FinalPath: resolved.Absolute, FinalContent: contentBytes, FinalExists: true, Original: patchSnapshot{Exists: false}}}, nil
}

func planDelete(operation patchOperation, config Config, policy PathPolicy) ([]patchChange, error) {
	resolved, err := policy.ResolveExisting(operation.Path)
	if err != nil {
		return nil, err
	}
	data, _, err := readTextFile(resolved.Absolute, config.MaxFileBytes)
	if err != nil {
		return nil, fmt.Errorf("delete %s: %w", resolved.Display, err)
	}
	return []patchChange{{Path: resolved, FinalPath: resolved.Absolute, FinalExists: false, Original: patchSnapshot{Exists: true, Content: data}}}, nil
}

func planUpdate(operation patchOperation, config Config, policy PathPolicy) ([]patchChange, error) {
	resolved, err := policy.ResolveExisting(operation.Path)
	if err != nil {
		return nil, err
	}
	original, currentHash, err := readTextFile(resolved.Absolute, config.MaxFileBytes)
	if err != nil {
		return nil, err
	}
	_ = currentHash
	updated, err := applyUpdateLines(string(original), operation.Lines)
	if err != nil {
		return nil, fmt.Errorf("update %s: %w", resolved.Display, err)
	}
	updatedBytes := []byte(updated)
	if err := validateWritableText(updatedBytes, config.MaxFileBytes); err != nil {
		return nil, err
	}
	return []patchChange{{Path: resolved, FinalPath: resolved.Absolute, FinalContent: updatedBytes, FinalExists: true, Original: patchSnapshot{Exists: true, Content: original}}}, nil
}

func planMoveUpdate(operation patchOperation, config Config, policy PathPolicy) ([]patchChange, error) {
	source, err := policy.ResolveExisting(operation.Path)
	if err != nil {
		return nil, err
	}
	target, err := policy.Resolve(operation.MoveTo)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(target.Absolute); err == nil {
		return nil, fmt.Errorf("move target already exists: %s", target.Display)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := ensureParentCreatable(target.Absolute); err != nil {
		return nil, err
	}
	original, _, err := readTextFile(source.Absolute, config.MaxFileBytes)
	if err != nil {
		return nil, err
	}
	final := original
	if hasPatchBody(operation.Lines) {
		updated, err := applyUpdateLines(string(original), operation.Lines)
		if err != nil {
			return nil, fmt.Errorf("update %s: %w", source.Display, err)
		}
		final = []byte(updated)
	}
	if err := validateWritableText(final, config.MaxFileBytes); err != nil {
		return nil, err
	}
	return []patchChange{
		{Path: source, FinalPath: source.Absolute, FinalExists: false, Original: patchSnapshot{Exists: true, Content: original}},
		{Path: target, FinalPath: target.Absolute, FinalContent: final, FinalExists: true, Original: patchSnapshot{Exists: false}},
	}, nil
}

func commitPatchChanges(changes []patchChange) ([]string, error) {
	applied := make([]patchChange, 0, len(changes))
	changedPaths := make([]string, 0, len(changes))
	for _, change := range changes {
		if err := applyPatchChange(change); err != nil {
			rollbackPatchChanges(append(applied, change))
			return nil, err
		}
		applied = append(applied, change)
		changedPaths = append(changedPaths, change.Path.Display)
	}
	return changedPaths, nil
}

func applyPatchChange(change patchChange) error {
	if change.FinalExists {
		return writeTextFile(change.FinalPath, change.FinalContent)
	}
	if err := os.Remove(change.FinalPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func rollbackPatchChanges(changes []patchChange) {
	for i := len(changes) - 1; i >= 0; i-- {
		change := changes[i]
		if change.Original.Exists {
			_ = writeTextFile(change.FinalPath, change.Original.Content)
			continue
		}
		_ = os.Remove(change.FinalPath)
	}
}

func hasPatchBody(lines []string) bool {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "@@") {
			return true
		}
	}
	return false
}

func addedPatchContent(lines []string) (string, error) {
	contentLines := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "+") {
			return "", fmt.Errorf("add file lines must start with +")
		}
		contentLines = append(contentLines, strings.TrimPrefix(line, "+"))
	}
	return strings.Join(contentLines, "\n") + "\n", nil
}

func applyUpdateLines(content string, lines []string) (string, error) {
	oldText, newText, err := updatePatchTexts(lines)
	if err != nil {
		return "", err
	}
	if oldText == "" {
		return "", fmt.Errorf("update patch has no removable or context text")
	}
	oldText, newText, err = matchPatchText(content, oldText, newText)
	if err != nil {
		return "", err
	}
	if strings.Count(content, oldText) != 1 {
		return "", fmt.Errorf("old patch text is ambiguous; provide more context")
	}
	return strings.Replace(content, oldText, newText, 1), nil
}

func matchPatchText(content string, oldText string, newText string) (string, string, error) {
	if strings.Contains(content, oldText) {
		return oldText, newText, nil
	}
	trimmedOld := strings.TrimSuffix(oldText, "\n")
	trimmedNew := strings.TrimSuffix(newText, "\n")
	if trimmedOld != oldText && trimmedOld != "" && strings.Contains(content, trimmedOld) {
		return trimmedOld, trimmedNew, nil
	}
	return "", "", fmt.Errorf("old patch text was not found")
}

func updatePatchTexts(lines []string) (string, string, error) {
	oldLines := []string{}
	newLines := []string{}
	for _, line := range lines {
		if strings.HasPrefix(line, "@@") || line == "" {
			continue
		}
		prefix := line[0]
		body := ""
		if len(line) > 1 {
			body = line[1:]
		}
		switch prefix {
		case ' ':
			oldLines = append(oldLines, body)
			newLines = append(newLines, body)
		case '-':
			oldLines = append(oldLines, body)
		case '+':
			newLines = append(newLines, body)
		default:
			return "", "", fmt.Errorf("update lines must start with space, -, +, or @@")
		}
	}
	oldText := strings.Join(oldLines, "\n")
	newText := strings.Join(newLines, "\n")
	if oldText != "" {
		oldText += "\n"
	}
	if newText != "" {
		newText += "\n"
	}
	return oldText, newText, nil
}
