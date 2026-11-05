// Package workspacehealth 探测工作区登记目录的运行时可用性。
// 登记事实只保存路径字符串；磁盘是否可达属于运行时健康，不参与数据合法性判定，
// 更不允许单个目录失联演变成整张工作区列表的读取失败。
package workspacehealth

import (
	"context"
	"os"
	"strings"

	"eucli-box/pkg/types"
)

// Probe 依次探测登记目录并汇总整体健康状态。
// 探测本身不返回错误：每个目录的失败都被降级为它自己的状态。
func Probe(ctx context.Context, directories []types.WorkspaceDirectory) types.WorkspaceHealth {
	health := types.WorkspaceHealth{
		Status:      types.WorkspaceHealthHealthy,
		Directories: make([]types.WorkspaceDirectoryHealth, 0, len(directories)),
	}
	for _, directory := range directories {
		item := ProbeDirectory(ctx, directory.Path)
		if item.Status != types.WorkspaceDirectoryAvailable {
			health.Status = types.WorkspaceHealthDegraded
		}
		health.Directories = append(health.Directories, item)
	}
	return health
}

// ProbeDirectory 探测单个目录。路径本身来自登记事实，这里只观察它当前是否可用。
func ProbeDirectory(ctx context.Context, rawPath string) types.WorkspaceDirectoryHealth {
	path := strings.TrimSpace(rawPath)
	item := types.WorkspaceDirectoryHealth{Path: path}
	if path == "" {
		item.Status = types.WorkspaceDirectoryUnavailable
		item.Detail = "workspace directory path is empty"
		return item
	}
	if err := ctx.Err(); err != nil {
		item.Status = types.WorkspaceDirectoryUnavailable
		item.Detail = err.Error()
		return item
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			item.Status = types.WorkspaceDirectoryMissing
			return item
		}
		item.Status = types.WorkspaceDirectoryUnavailable
		item.Detail = err.Error()
		return item
	}
	if !info.IsDir() {
		item.Status = types.WorkspaceDirectoryNotDirectory
		return item
	}
	item.Status = types.WorkspaceDirectoryAvailable
	return item
}
