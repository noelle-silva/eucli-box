package fileoperator

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func readTextFile(path string, maxFileBytes int64) ([]byte, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", err
	}
	if info.IsDir() {
		return nil, "", fmt.Errorf("path is a directory")
	}
	if info.Size() > maxFileBytes {
		return nil, "", fmt.Errorf("file is too large: %d bytes exceeds %d bytes", info.Size(), maxFileBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	if isBinary(data) {
		return nil, "", fmt.Errorf("binary files are not readable as text")
	}
	return data, hashBytes(data), nil
}

func writeTextFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func validateWritableText(content []byte, maxFileBytes int64) error {
	if int64(len(content)) > maxFileBytes {
		return fmt.Errorf("content is too large: %d bytes exceeds %d bytes", len(content), maxFileBytes)
	}
	if containsNullByte(content) {
		return fmt.Errorf("content cannot contain binary data")
	}
	return nil
}

func containsNullByte(data []byte) bool {
	for _, value := range data {
		if value == 0 {
			return true
		}
	}
	return false
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func isBinary(data []byte) bool {
	limit := len(data)
	if limit > 8192 {
		limit = 8192
	}
	for i := 0; i < limit; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

func splitLines(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	if normalized == "" {
		return []string{}
	}
	lines := strings.Split(normalized, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func ensureExpectedHash(path string, expectedHash string, maxFileBytes int64) (string, error) {
	expectedHash = strings.TrimSpace(expectedHash)
	if expectedHash == "" {
		return "", nil
	}
	data, currentHash, err := readTextFile(path, maxFileBytes)
	if err != nil {
		return "", err
	}
	_ = data
	if currentHash != expectedHash {
		return currentHash, fmt.Errorf("file changed since expectedHash; current hash is %s", currentHash)
	}
	return currentHash, nil
}

func isHiddenName(name string) bool {
	return strings.HasPrefix(name, ".")
}

func displayPath(base string, target string) string {
	if rel, err := filepath.Rel(base, target); err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
		return filepath.ToSlash(rel)
	}
	return target
}
