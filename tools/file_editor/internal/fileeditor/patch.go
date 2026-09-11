package fileeditor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/pkg/types"
)

type patchOperation struct {
	Kind       string
	Path       string
	MoveTo     string
	AddContent []string
	Hunks      []patchHunk
}

type patchHunk struct {
	Lines []patchHunkLine
}

type patchHunkLine struct {
	Kind byte
	Text string
}

type patchSection struct {
	Kind   string
	Path   string
	MoveTo string
	Body   []string
}

type patchChange struct {
	Path         ResolvedPath
	FinalPath    string
	FinalContent []byte
	FinalExists  bool
	Original     patchSnapshot
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
	facts := []resultFact{
		intFact("operations", len(operations)),
		intFact("changedPaths", len(changedPaths)),
	}
	return success(appendEnvelope(fmt.Sprintf("Applied patch to %d operation(s).", len(operations)), "apply_patch", facts), metadata)
}

func parsePatchText(patchText string) ([]patchOperation, error) {
	lines := splitPatchLines(patchText)
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "*** Begin Patch" {
		return nil, fmt.Errorf("patch must start with *** Begin Patch")
	}
	operations := []patchOperation{}
	var current *patchSection
	finished := false
	flush := func() error {
		if current == nil {
			return nil
		}
		operation, err := finalizePatchSection(*current)
		if err != nil {
			return err
		}
		operations = append(operations, operation)
		current = nil
		return nil
	}
	for index := 1; index < len(lines); index++ {
		line := lines[index]
		trimmed := strings.TrimSpace(line)
		if finished {
			if trimmed != "" {
				return nil, fmt.Errorf("patch content appeared after *** End Patch at line %d", index+1)
			}
			continue
		}
		if trimmed == "*** End Patch" {
			if err := flush(); err != nil {
				return nil, err
			}
			finished = true
			continue
		}
		if strings.HasPrefix(line, "*** Add File: ") || strings.HasPrefix(line, "*** Delete File: ") || strings.HasPrefix(line, "*** Update File: ") {
			if err := flush(); err != nil {
				return nil, err
			}
			section, err := parsePatchHeader(line)
			if err != nil {
				return nil, err
			}
			current = &section
			continue
		}
		if current == nil {
			if trimmed == "" {
				continue
			}
			return nil, fmt.Errorf("patch content appeared before a file header at line %d", index+1)
		}
		if strings.HasPrefix(line, "*** Move to: ") {
			if current.Kind != "update" {
				return nil, fmt.Errorf("*** Move to is only allowed in an Update File section")
			}
			if current.MoveTo != "" {
				return nil, fmt.Errorf("duplicate *** Move to line in %s", current.Path)
			}
			moveTo := strings.TrimSpace(strings.TrimPrefix(line, "*** Move to: "))
			if moveTo == "" {
				return nil, fmt.Errorf("move target path is required")
			}
			current.MoveTo = moveTo
			continue
		}
		current.Body = append(current.Body, line)
	}
	if !finished {
		return nil, fmt.Errorf("patch must end with *** End Patch")
	}
	return operations, nil
}

func splitPatchLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Split(text, "\n")
}

func parsePatchHeader(line string) (patchSection, error) {
	switch {
	case strings.HasPrefix(line, "*** Add File: "):
		path := strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: "))
		if path == "" {
			return patchSection{}, fmt.Errorf("add file path is required")
		}
		return patchSection{Kind: "add", Path: path}, nil
	case strings.HasPrefix(line, "*** Delete File: "):
		path := strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File: "))
		if path == "" {
			return patchSection{}, fmt.Errorf("delete file path is required")
		}
		return patchSection{Kind: "delete", Path: path}, nil
	case strings.HasPrefix(line, "*** Update File: "):
		path := strings.TrimSpace(strings.TrimPrefix(line, "*** Update File: "))
		if path == "" {
			return patchSection{}, fmt.Errorf("update file path is required")
		}
		return patchSection{Kind: "update", Path: path}, nil
	default:
		return patchSection{}, fmt.Errorf("invalid patch file header")
	}
}

// finalizePatchSection validates one raw section and turns it into a patch
// operation. Trailing blank lines are formatting; interior blank lines in an
// update body are empty context lines.
func finalizePatchSection(section patchSection) (patchOperation, error) {
	body := section.Body
	for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
		body = body[:len(body)-1]
	}
	operation := patchOperation{Kind: section.Kind, Path: section.Path, MoveTo: section.MoveTo}
	switch section.Kind {
	case "add":
		for _, line := range body {
			if !strings.HasPrefix(line, "+") {
				return patchOperation{}, fmt.Errorf("add file lines must start with +: %q", line)
			}
			operation.AddContent = append(operation.AddContent, strings.TrimPrefix(line, "+"))
		}
	case "delete":
		if len(body) > 0 {
			return patchOperation{}, fmt.Errorf("delete file section cannot carry content: %s", section.Path)
		}
	case "update":
		hunks, err := parseUpdateHunks(body)
		if err != nil {
			return patchOperation{}, fmt.Errorf("update %s: %w", section.Path, err)
		}
		operation.Hunks = hunks
		if len(hunks) == 0 && operation.MoveTo == "" {
			return patchOperation{}, fmt.Errorf("update file section has no changes: %s", section.Path)
		}
	default:
		return patchOperation{}, fmt.Errorf("unsupported patch operation %q", section.Kind)
	}
	return operation, nil
}

func parseUpdateHunks(body []string) ([]patchHunk, error) {
	hunks := []patchHunk{}
	var current []patchHunkLine
	flush := func() error {
		if current == nil {
			return nil
		}
		if len(current) == 0 {
			return fmt.Errorf("@@ section has no change lines")
		}
		hunks = append(hunks, patchHunk{Lines: current})
		current = nil
		return nil
	}
	for _, line := range body {
		if strings.HasPrefix(line, "@@") {
			if strings.TrimSpace(strings.TrimPrefix(line, "@@")) != "" {
				return nil, fmt.Errorf("@@ separator must not carry content")
			}
			if err := flush(); err != nil {
				return nil, err
			}
			current = []patchHunkLine{}
			continue
		}
		parsed, err := parseUpdateHunkLine(line)
		if err != nil {
			return nil, err
		}
		current = append(current, parsed)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return hunks, nil
}

func parseUpdateHunkLine(line string) (patchHunkLine, error) {
	if line == "" {
		return patchHunkLine{Kind: ' ', Text: ""}, nil
	}
	switch line[0] {
	case ' ':
		return patchHunkLine{Kind: ' ', Text: line[1:]}, nil
	case '-':
		return patchHunkLine{Kind: '-', Text: line[1:]}, nil
	case '+':
		return patchHunkLine{Kind: '+', Text: line[1:]}, nil
	default:
		return patchHunkLine{}, fmt.Errorf("update lines must start with space, -, +, or @@")
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
	contentBytes := []byte(addedPatchContent(operation.AddContent))
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
	original, _, err := readTextFile(resolved.Absolute, config.MaxFileBytes)
	if err != nil {
		return nil, err
	}
	updated, err := applyUpdateLines(string(original), operation.Hunks)
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
	if len(operation.Hunks) > 0 {
		updated, err := applyUpdateLines(string(original), operation.Hunks)
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

// applyUpdateLines applies each hunk independently and in order. A hunk is
// located after the previous hunk's replacement and must be unique there.
func applyUpdateLines(content string, hunks []patchHunk) (string, error) {
	style := detectLineEnding([]byte(content))
	searchStart := 0
	for index, hunk := range hunks {
		oldText, newText := hunkTexts(hunk)
		oldText = applyLineEndingStyle(oldText, style)
		newText = applyLineEndingStyle(newText, style)
		if oldText == "" {
			return "", fmt.Errorf("patch section %d has no removable or context text", index+1)
		}
		area := content[searchStart:]
		matchedOld, matchedNew, err := matchPatchText(area, oldText, newText)
		if err != nil {
			return "", fmt.Errorf("patch section %d: %w", index+1, err)
		}
		found := strings.Index(area, matchedOld)
		if strings.Contains(area[found+1:], matchedOld) {
			return "", fmt.Errorf("patch section %d: old patch text is ambiguous; provide more context", index+1)
		}
		absolute := searchStart + found
		content = content[:absolute] + matchedNew + content[absolute+len(matchedOld):]
		searchStart = absolute + len(matchedNew)
	}
	return content, nil
}

// matchPatchText locates the hunk text in the search area. When the hunk text
// carries a trailing newline but the file ends without one, the trimmed form
// is used instead.
func matchPatchText(searchArea string, oldText string, newText string) (string, string, error) {
	if strings.Contains(searchArea, oldText) {
		return oldText, newText, nil
	}
	trimmedOld := strings.TrimSuffix(oldText, "\n")
	trimmedNew := strings.TrimSuffix(newText, "\n")
	if trimmedOld != oldText && trimmedOld != "" && strings.Contains(searchArea, trimmedOld) {
		return trimmedOld, trimmedNew, nil
	}
	return "", "", fmt.Errorf("old patch text was not found")
}

func hunkTexts(hunk patchHunk) (string, string) {
	oldLines := []string{}
	newLines := []string{}
	for _, line := range hunk.Lines {
		switch line.Kind {
		case ' ':
			oldLines = append(oldLines, line.Text)
			newLines = append(newLines, line.Text)
		case '-':
			oldLines = append(oldLines, line.Text)
		case '+':
			newLines = append(newLines, line.Text)
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
	return oldText, newText
}

func commitPatchChanges(changes []patchChange) ([]string, error) {
	applied := make([]patchChange, 0, len(changes))
	changedPaths := make([]string, 0, len(changes))
	for _, change := range changes {
		if err := applyPatchChange(change); err != nil {
			failed := append(append([]patchChange{}, applied...), change)
			if rollbackErr := rollbackPatchChanges(failed); rollbackErr != nil {
				return nil, fmt.Errorf("%w (rollback also failed: %v)", err, rollbackErr)
			}
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

func rollbackPatchChanges(changes []patchChange) error {
	failures := []error{}
	for i := len(changes) - 1; i >= 0; i-- {
		change := changes[i]
		if change.Original.Exists {
			if err := writeTextFile(change.FinalPath, change.Original.Content); err != nil {
				failures = append(failures, fmt.Errorf("restore %s: %w", change.FinalPath, err))
			}
			continue
		}
		if err := os.Remove(change.FinalPath); err != nil && !os.IsNotExist(err) {
			failures = append(failures, fmt.Errorf("remove %s: %w", change.FinalPath, err))
		}
	}
	return errors.Join(failures...)
}

func addedPatchContent(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
