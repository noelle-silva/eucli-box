// Package workspace 提供统一开发工作区的路径事实。
// 所有本机开发资料（手动体验、发布、验证、凭据）都从同一个工作区根派生，
// 任何入口不得自行拼接工作区路径。
package workspace

import "path/filepath"

const (
	// WorkspaceDirectory 是项目根下统一开发工作区的目录名。
	WorkspaceDirectory = ".dev-workspace"
	// ReleaseDirectory 是工作区内发布与验证资料的目录名。
	ReleaseDirectory = ".release"
	// RuntimeDirectory 是工作区内手动开发体验资料的目录名。
	RuntimeDirectory = ".dev-runtime"
)

// RelativeReleaseRoot 是发布根相对项目根的路径，用于报错展示等场景。
const RelativeReleaseRoot = WorkspaceDirectory + "/" + ReleaseDirectory

// RelativeCredentialsPath 是发布凭据文件相对项目根的路径。
const RelativeCredentialsPath = RelativeReleaseRoot + "/config/github.env"

// Root 返回统一开发工作区根。
func Root(repositoryRoot string) string {
	return filepath.Join(repositoryRoot, WorkspaceDirectory)
}

// ReleaseRoot 返回发布与验证资料根。
func ReleaseRoot(repositoryRoot string) string {
	return filepath.Join(Root(repositoryRoot), ReleaseDirectory)
}

// RuntimeRoot 返回手动开发体验资料根。
func RuntimeRoot(repositoryRoot string) string {
	return filepath.Join(Root(repositoryRoot), RuntimeDirectory)
}

// WorkRoot 返回发布临时现场根。
func WorkRoot(repositoryRoot string) string {
	return filepath.Join(ReleaseRoot(repositoryRoot), "work")
}

// OutputRoot 返回已核对本地成品根。
func OutputRoot(repositoryRoot string) string {
	return filepath.Join(ReleaseRoot(repositoryRoot), "output")
}

// LogsRoot 返回脱敏发布日志根。
func LogsRoot(repositoryRoot string) string {
	return filepath.Join(ReleaseRoot(repositoryRoot), "logs")
}

// VerificationRoot 返回阶段自动验证根。
func VerificationRoot(repositoryRoot string) string {
	return filepath.Join(ReleaseRoot(repositoryRoot), "verification")
}

// VerificationStageRoot 返回指定阶段的验证隔离根。
func VerificationStageRoot(repositoryRoot string, stage string) string {
	return filepath.Join(VerificationRoot(repositoryRoot), "stage-"+stage)
}

// VerificationCacheRoot 返回验证区公共缓存根。
// 依赖模块与编译缓存属于跨运行可复用的已验证输入，不属于任何一次运行现场。
func VerificationCacheRoot(repositoryRoot string) string {
	return filepath.Join(VerificationRoot(repositoryRoot), "cache")
}

// AssetRoot 返回发布与构建的已验证资产根。
// 内含 prepared（准备产物）、cache（输入文件）与 temp（解压临时）三兄弟目录。
func AssetRoot(repositoryRoot string) string {
	return filepath.Join(ReleaseRoot(repositoryRoot), "assets")
}

// VerificationAssetRoot 返回验证的已验证资产根。
// 与验证公共缓存同族，内含 prepared、cache 与 temp 三兄弟目录。
func VerificationAssetRoot(repositoryRoot string) string {
	return filepath.Join(VerificationCacheRoot(repositoryRoot), "assets")
}

// CredentialsPath 返回本地发布凭据文件路径。
func CredentialsPath(repositoryRoot string) string {
	return filepath.Join(ReleaseRoot(repositoryRoot), "config", "github.env")
}
