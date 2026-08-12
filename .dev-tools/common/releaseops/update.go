package releaseops

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"eucli-box/pkg/release"
)

type UpdateResult struct {
	Target          string
	PreviousVersion string
	Version         string
	Warning         string
}

type committedCleanupError struct {
	err error
}

func (e *committedCleanupError) Error() string {
	return "版本已调整并通过完整检查，但清理临时备份失败：" + e.err.Error()
}

func (e *committedCleanupError) Unwrap() error {
	return e.err
}

type fileChange struct {
	path      string
	payload   []byte
	mode      os.FileMode
	tempPath  string
	backup    string
	committed bool
}

func SetVersion(root string, target string, nextVersion string, message string) (UpdateResult, error) {
	artifact, err := Resolve(root, target)
	if err != nil {
		return UpdateResult{}, err
	}
	if err := Check(artifact); err != nil {
		return UpdateResult{}, fmt.Errorf("调整前完整检查失败：%w", err)
	}
	nextVersion = strings.TrimSpace(nextVersion)
	if err := release.ValidateVersion(nextVersion); err != nil {
		return UpdateResult{}, fmt.Errorf("新版本无效：%w", err)
	}
	order, err := release.CompareVersions(nextVersion, artifact.Version)
	if err != nil {
		return UpdateResult{}, err
	}
	if order <= 0 {
		return UpdateResult{}, fmt.Errorf("新版本 %s 必须高于当前版本 %s", nextVersion, artifact.Version)
	}
	message, err = validateUpdateMessage(message)
	if err != nil {
		return UpdateResult{}, err
	}

	changes, err := prepareVersionChanges(artifact, nextVersion, message)
	if err != nil {
		return UpdateResult{}, err
	}
	result := UpdateResult{Target: artifact.Target(), PreviousVersion: artifact.Version, Version: nextVersion}
	if err := applyChanges(changes, func() error {
		updated, err := Resolve(root, target)
		if err != nil {
			return err
		}
		return Check(updated)
	}); err != nil {
		var cleanupErr *committedCleanupError
		if errors.As(err, &cleanupErr) {
			result.Warning = cleanupErr.Error()
			return result, nil
		}
		return UpdateResult{}, fmt.Errorf("版本调整失败：%w", err)
	}
	return result, nil
}

func prepareVersionChanges(artifact Artifact, nextVersion string, message string) ([]fileChange, error) {
	changes := make([]fileChange, 0, 7)
	metadata, err := replaceTopLevelJSONVersionFile(artifact.MetadataPath, artifact.Version, nextVersion)
	if err != nil {
		return nil, err
	}
	changes = append(changes, metadata)

	changelog, err := prependChangelogVersion(artifact.ChangelogPath, nextVersion, message)
	if err != nil {
		return nil, err
	}
	changes = append(changes, changelog)

	if artifact.Kind == KindClient {
		paths := clientVersionFiles(artifact.Directory)
		for _, path := range paths.jsonFiles {
			change, err := replaceTopLevelJSONVersionFile(path, artifact.Version, nextVersion)
			if err != nil {
				return nil, err
			}
			changes = append(changes, change)
		}
		for _, source := range []struct {
			path     string
			selector tomlVersionSelector
		}{
			{path: paths.cargoTOML, selector: packageVersion},
			{path: paths.cargoLock, selector: aiStudioLockVersion},
		} {
			change, err := replaceTOMLVersionFile(source.path, source.selector, artifact.Version, nextVersion)
			if err != nil {
				return nil, err
			}
			changes = append(changes, change)
		}
	}
	return changes, nil
}

func validateUpdateMessage(message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return "", fmt.Errorf("必须提供本次版本的中文更新说明")
	}
	if strings.ContainsAny(message, "\r\n") {
		return "", fmt.Errorf("更新说明必须是单行中文说明")
	}
	for _, value := range message {
		if unicode.Is(unicode.Han, value) {
			return message, nil
		}
	}
	return "", fmt.Errorf("更新说明必须使用中文")
}

func replaceTopLevelJSONVersionFile(path string, previous string, next string) (fileChange, error) {
	payload, mode, err := readChangeSource(path)
	if err != nil {
		return fileChange{}, err
	}
	updated, err := replaceTopLevelJSONVersion(payload, previous, next)
	if err != nil {
		return fileChange{}, fmt.Errorf("更新 %s 失败：%w", path, err)
	}
	return fileChange{path: path, payload: updated, mode: mode}, nil
}

func replaceTopLevelJSONVersion(payload []byte, previous string, next string) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, fmt.Errorf("发布资料必须是 JSON 对象")
	}
	matchStart := -1
	matchEnd := -1
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, fmt.Errorf("JSON 对象字段名无效")
		}
		beforeValue := decoder.InputOffset()
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, err
		}
		if key != "version" {
			continue
		}
		if matchStart >= 0 {
			return nil, fmt.Errorf("存在重复的顶层 version 字段")
		}
		var version string
		if err := json.Unmarshal(raw, &version); err != nil {
			return nil, fmt.Errorf("顶层 version 必须是字符串")
		}
		if strings.TrimSpace(version) != previous {
			return nil, fmt.Errorf("version 为 %q，预期为 %q", version, previous)
		}
		rawStart := int(beforeValue)
		rawEnd := int(decoder.InputOffset())
		segment := payload[rawStart:rawEnd]
		encodedPrevious, _ := json.Marshal(version)
		relative := bytes.LastIndex(segment, encodedPrevious)
		if relative < 0 {
			return nil, fmt.Errorf("无法定位 version 字段值")
		}
		matchStart = rawStart + relative
		matchEnd = matchStart + len(encodedPrevious)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	if matchStart < 0 {
		return nil, fmt.Errorf("缺少顶层 version 字段")
	}
	encodedNext, _ := json.Marshal(next)
	updated := make([]byte, 0, len(payload)-matchEnd+matchStart+len(encodedNext))
	updated = append(updated, payload[:matchStart]...)
	updated = append(updated, encodedNext...)
	updated = append(updated, payload[matchEnd:]...)
	return updated, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("JSON 后存在多余内容")
		}
		return err
	}
	return nil
}

func replaceTOMLVersionFile(path string, selector tomlVersionSelector, previous string, next string) (fileChange, error) {
	payload, mode, err := readChangeSource(path)
	if err != nil {
		return fileChange{}, err
	}
	updated, err := replaceTOMLVersion(string(payload), selector, previous, next)
	if err != nil {
		return fileChange{}, fmt.Errorf("更新 %s 失败：%w", path, err)
	}
	return fileChange{path: path, payload: []byte(updated), mode: mode}, nil
}

func replaceTOMLVersion(source string, selector tomlVersionSelector, previous string, next string) (string, error) {
	lineEnding := "\n"
	if strings.Contains(source, "\r\n") {
		lineEnding = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	insidePackage := false
	matchedName := false
	replaced := false
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch selector {
		case packageVersion:
			if strings.HasPrefix(trimmed, "[") {
				insidePackage = trimmed == "[package]"
				continue
			}
		case aiStudioLockVersion:
			if trimmed == "[[package]]" {
				insidePackage = true
				matchedName = false
				continue
			}
			if insidePackage {
				if name, ok := parseTOMLStringAssignment(trimmed, "name"); ok {
					matchedName = name == "ai-studio-app"
					continue
				}
			}
		}
		eligible := selector == packageVersion && insidePackage || selector == aiStudioLockVersion && insidePackage && matchedName
		if !eligible {
			continue
		}
		version, ok := parseTOMLStringAssignment(trimmed, "version")
		if !ok {
			continue
		}
		if replaced {
			return "", fmt.Errorf("存在重复的目标版本字段")
		}
		if version != previous {
			return "", fmt.Errorf("版本为 %q，预期为 %q", version, previous)
		}
		prefixLength := len(line) - len(strings.TrimLeft(line, " \t"))
		lines[index] = line[:prefixLength] + `version = "` + next + `"`
		replaced = true
	}
	if !replaced {
		return "", fmt.Errorf("未找到目标版本字段")
	}
	return strings.Join(lines, lineEnding), nil
}

func prependChangelogVersion(path string, version string, message string) (fileChange, error) {
	payload, mode, err := readChangeSource(path)
	if err != nil {
		return fileChange{}, err
	}
	source := string(payload)
	lineEnding := "\n"
	if strings.Contains(source, "\r\n") {
		lineEnding = "\r\n"
	}
	normalized := strings.ReplaceAll(source, "\r\n", "\n")
	heading := regexp.MustCompile(`(?m)^##\s+` + regexp.QuoteMeta(version) + `(?:\s|$)`)
	if heading.MatchString(normalized) {
		return fileChange{}, fmt.Errorf("%s 已存在版本 %s 的更新记录", path, version)
	}
	firstLineEnd := strings.IndexByte(normalized, '\n')
	if firstLineEnd < 0 || !strings.HasPrefix(strings.TrimSpace(normalized[:firstLineEnd]), "# ") {
		return fileChange{}, fmt.Errorf("%s 的 CHANGELOG 必须以一级标题开始", path)
	}
	entry := fmt.Sprintf("\n\n## %s - %s\n\n- %s", version, time.Now().Format("2006-01-02"), message)
	updated := normalized[:firstLineEnd] + entry + normalized[firstLineEnd:]
	if lineEnding == "\r\n" {
		updated = strings.ReplaceAll(updated, "\n", "\r\n")
	}
	return fileChange{path: path, payload: []byte(updated), mode: mode}, nil
}

func readChangeSource(path string) ([]byte, os.FileMode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, fmt.Errorf("读取 %s 失败：%w", path, err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, fmt.Errorf("读取 %s 失败：%w", path, err)
	}
	return payload, info.Mode(), nil
}

func applyChanges(changes []fileChange, verify func() error) error {
	if len(changes) == 0 {
		return fmt.Errorf("没有可写入的版本变化")
	}
	seen := map[string]struct{}{}
	for index := range changes {
		path := filepath.Clean(changes[index].path)
		if _, exists := seen[path]; exists {
			return fmt.Errorf("重复写入文件 %s", path)
		}
		seen[path] = struct{}{}
		temp, err := os.CreateTemp(filepath.Dir(path), ".eucli-version-*.tmp")
		if err != nil {
			cleanupStaged(changes)
			return err
		}
		changes[index].tempPath = temp.Name()
		if err := temp.Chmod(changes[index].mode); err != nil {
			_ = temp.Close()
			cleanupStaged(changes)
			return err
		}
		if _, err := temp.Write(changes[index].payload); err != nil {
			_ = temp.Close()
			cleanupStaged(changes)
			return err
		}
		if err := temp.Sync(); err != nil {
			_ = temp.Close()
			cleanupStaged(changes)
			return err
		}
		if err := temp.Close(); err != nil {
			cleanupStaged(changes)
			return err
		}
	}

	for index := range changes {
		backup, err := reserveBackupPath(filepath.Dir(changes[index].path))
		if err != nil {
			return rollbackChanges(changes, err)
		}
		changes[index].backup = backup
		if err := os.Rename(changes[index].path, backup); err != nil {
			return rollbackChanges(changes, err)
		}
		if err := os.Rename(changes[index].tempPath, changes[index].path); err != nil {
			restoreErr := os.Rename(backup, changes[index].path)
			if restoreErr == nil {
				changes[index].backup = ""
			} else {
				changes[index].committed = true
			}
			return rollbackChanges(changes, errors.Join(err, restoreErr))
		}
		changes[index].tempPath = ""
		changes[index].committed = true
	}
	if err := verify(); err != nil {
		return rollbackChanges(changes, fmt.Errorf("写入后完整检查失败：%w", err))
	}
	cleanupErrors := make([]error, 0)
	for index := range changes {
		if changes[index].backup != "" {
			if err := os.Remove(changes[index].backup); err != nil {
				cleanupErrors = append(cleanupErrors, err)
				continue
			}
			changes[index].backup = ""
		}
	}
	if len(cleanupErrors) > 0 {
		return &committedCleanupError{err: errors.Join(cleanupErrors...)}
	}
	return nil
}

func reserveBackupPath(directory string) (string, error) {
	file, err := os.CreateTemp(directory, ".eucli-version-backup-*")
	if err != nil {
		return "", err
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(name); err != nil {
		return "", err
	}
	return name, nil
}

func rollbackChanges(changes []fileChange, cause error) error {
	errorsFound := []error{cause}
	for index := len(changes) - 1; index >= 0; index-- {
		change := &changes[index]
		if change.committed {
			if err := os.Remove(change.path); err != nil && !os.IsNotExist(err) {
				errorsFound = append(errorsFound, err)
				continue
			}
			if err := os.Rename(change.backup, change.path); err != nil {
				errorsFound = append(errorsFound, err)
			} else {
				change.backup = ""
				change.committed = false
			}
		}
	}
	cleanupStaged(changes)
	return errors.Join(errorsFound...)
}

func cleanupStaged(changes []fileChange) {
	for index := range changes {
		if changes[index].tempPath != "" {
			_ = os.Remove(changes[index].tempPath)
		}
		if changes[index].backup != "" && !changes[index].committed {
			_ = os.Remove(changes[index].backup)
		}
	}
}
