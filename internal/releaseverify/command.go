package releaseverify

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func runCommand(ctx context.Context, paths runPaths, name string, workdir string, command string, args ...string) error {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = workdir
	cmd.Env = verificationEnvironment(os.Environ(), paths)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	logPath := filepath.Join(paths.evidence, evidenceLogFileName(name))
	payload := append([]byte("stdout:\n"), stdout.Bytes()...)
	payload = append(payload, []byte("\nstderr:\n")...)
	payload = append(payload, stderr.Bytes()...)
	if writeErr := os.WriteFile(logPath, payload, 0o644); writeErr != nil {
		return writeErr
	}
	if err != nil {
		return fmt.Errorf("%w：%s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func verificationEnvironment(base []string, paths runPaths) []string {
	replacements := map[string]string{
		"TEMP":         paths.temp,
		"TMP":          paths.temp,
		"GOCACHE":      filepath.Join(paths.cache, "go-build"),
		"GOMODCACHE":   filepath.Join(paths.cache, "go-mod"),
		"GOTMPDIR":     filepath.Join(paths.temp, "go"),
		"GOTELEMETRY":  "off",
		"GH_TOKEN":     "",
		"GITHUB_TOKEN": "",
	}
	for _, path := range []string{replacements["GOCACHE"], replacements["GOMODCACHE"], replacements["GOTMPDIR"]} {
		_ = os.MkdirAll(path, 0o755)
	}
	result := make([]string, 0, len(base)+len(replacements))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if ok {
			if _, replaced := replacements[strings.ToUpper(key)]; replaced {
				continue
			}
		}
		result = append(result, item)
	}
	for key, value := range replacements {
		result = append(result, key+"="+value)
	}
	return result
}

func evidenceLogFileName(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastWasSeparator := false
	for _, char := range normalized {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-' || char == '_' {
			if builder.Len() >= 40 {
				break
			}
			builder.WriteRune(char)
			lastWasSeparator = char == '-' || char == '_'
		} else if builder.Len() > 0 && !lastWasSeparator {
			builder.WriteByte('-')
			lastWasSeparator = true
		}
	}
	prefix := strings.Trim(builder.String(), "-_")
	if prefix == "" {
		prefix = "check"
	}
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return prefix + "-" + hex.EncodeToString(digest[:]) + ".log"
}
