package gateway

import (
	"io"
	"net/http"
	"os"
)

// maxArtifactImportBytes 是导入包上传的大小上限：正常成品远小于该值，超限快速失败。
const maxArtifactImportBytes = 512 << 20

// handleImportTool 接收工具包上传并交给工具系统直接导入。
func (s *system) handleImportTool(w http.ResponseWriter, r *http.Request) {
	archivePath, cleanup, err := receiveImportArchive(w, r)
	if err != nil {
		writeError(w, err)
		return
	}
	defer cleanup()
	state, err := s.tools.ImportToolPackage(r.Context(), archivePath)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, state)
}

// handleImportPlugin 接收插件包上传并交给系统插件系统直接导入。
func (s *system) handleImportPlugin(w http.ResponseWriter, r *http.Request) {
	archivePath, cleanup, err := receiveImportArchive(w, r)
	if err != nil {
		writeError(w, err)
		return
	}
	defer cleanup()
	state, err := s.systemPlugins.ImportPluginPackage(r.Context(), archivePath)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, state)
}

// receiveImportArchive 把上传的压缩包落到临时文件；超限或传输中断时快速失败。
func receiveImportArchive(w http.ResponseWriter, r *http.Request) (string, func(), error) {
	file, err := os.CreateTemp("", "eucli-box-import-*.zip")
	if err != nil {
		return "", nil, gatewayDependencyFailed("无法建立导入临时文件", err)
	}
	limited := http.MaxBytesReader(w, r.Body, maxArtifactImportBytes)
	if _, err := io.Copy(file, limited); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", nil, gatewayInvalid("导入包上传失败（超出大小上限或传输中断）", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", nil, gatewayDependencyFailed("导入临时文件写入失败", err)
	}
	return file.Name(), func() { _ = os.Remove(file.Name()) }, nil
}
