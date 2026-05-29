package toolcalling

import (
	"os"
	"runtime"
	"strings"

	"eucli-box/pkg/types"
)

func selectExecutable(tool types.ToolDefinition) (string, error) {
	for _, binary := range tool.Binaries {
		if binary.GOOS == runtime.GOOS && binary.GOARCH == runtime.GOARCH {
			if strings.TrimSpace(binary.Path) == "" {
				return "", toolInvalid("tool binary path is required", nil)
			}
			executable, err := cleanExecutablePath(tool, binary.Path)
			if err != nil {
				return "", err
			}
			info, err := os.Stat(executable)
			if err != nil {
				return "", toolNotFound("tool executable does not exist", err)
			}
			if info.IsDir() {
				return "", toolInvalid("tool executable is a directory", nil)
			}
			return executable, nil
		}
	}
	return "", toolNotFound("no tool executable matches current platform", nil)
}
