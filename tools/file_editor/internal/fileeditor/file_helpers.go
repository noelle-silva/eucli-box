package fileeditor

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

// writeTextFile writes through a same-directory temporary file and renames it
// into place, so a failure never leaves a half-written target and the target's
// permissions are preserved.
func writeTextFile(path string, data []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	temp, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempName)
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	if err := os.Chmod(tempName, mode); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	return nil
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

// verifyExpectedHash enforces the optional expected-hash guard. An expected
// hash is a statement that the target must exist with exactly that content,
// so a missing target is an error and a mismatch reports the real hash.
func verifyExpectedHash(expected string, targetExists bool, currentHash string) (string, error) {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return "", nil
	}
	if !targetExists {
		return "", fmt.Errorf("expectedHash was provided but the target file does not exist")
	}
	if currentHash != expected {
		return currentHash, fmt.Errorf("file changed since expectedHash; current hash is %s", currentHash)
	}
	return currentHash, nil
}
