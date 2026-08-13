package datamigration

import (
	"os"
	"path/filepath"
)

// WorkspaceDir 返回数据目录旁的迁移工作区路径。
// 数据目录先规范化为绝对路径，工作区固定为其兄弟目录 <数据目录名>.migration。
func WorkspaceDir(dataDir string) string {
	absolute, err := filepath.Abs(dataDir)
	if err != nil {
		return ""
	}
	clean := filepath.Clean(absolute)
	return filepath.Join(filepath.Dir(clean), filepath.Base(clean)+".migration")
}

// workspace 是迁移工作区的路径集合。
type workspace struct {
	dir string
}

func newWorkspace(dataDir string) (workspace, error) {
	absolute, err := filepath.Abs(dataDir)
	if err != nil {
		return workspace{}, migrationPrepareFailed("failed to resolve data directory", err)
	}
	clean := filepath.Clean(absolute)
	return workspace{dir: filepath.Join(filepath.Dir(clean), filepath.Base(clean)+".migration")}, nil
}

func (w workspace) statusFile() string {
	return filepath.Join(w.dir, "status.json")
}

func (w workspace) processFile() string {
	return filepath.Join(w.dir, "process.json")
}

func (w workspace) backupRoot() string {
	return filepath.Join(w.dir, "backup")
}

func (w workspace) backupRunDir(runID string) string {
	return filepath.Join(w.backupRoot(), "run-"+runID)
}

func (w workspace) backupDataDir(runID string) string {
	return filepath.Join(w.backupRunDir(runID), "data")
}

func (w workspace) manifestFile(runID string) string {
	return filepath.Join(w.backupRunDir(runID), "manifest.json")
}

// ensure 建立工作区目录；工作区永远不在数据目录内部。
func (w workspace) ensure() error {
	if err := os.MkdirAll(w.dir, 0o700); err != nil {
		return migrationPrepareFailed("failed to create migration workspace", err)
	}
	return nil
}
