//go:build !windows

package datamigration

// checkDiskSpace 首期只发布 Windows x64，其他平台不参与空间判断。
func checkDiskSpace(_ string, _ uint64) error {
	return nil
}
