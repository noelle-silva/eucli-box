package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"eucli-box/pkg/localrun"
)

type localBoxInstallWorkState struct {
	SchemaVersion int              `json:"schemaVersion"`
	Status        string           `json:"status"`
	Progress      localBoxProgress `json:"progress"`
	Error         localBoxError    `json:"error"`
}

func writeLocalBoxInstallWorkState(workDir string, state localBoxState) error {
	value := localBoxInstallWorkState{
		SchemaVersion: 1,
		Status:        state.Status,
		Progress:      state.Progress,
		Error:         state.Error,
	}
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return fmt.Errorf("建立安装阶段目录失败：%w", err)
	}
	if err := localrun.WritePrivateJSON(filepath.Join(workDir, "state.json"), value); err != nil {
		return fmt.Errorf("写入安装阶段资料失败：%w", err)
	}
	return nil
}

type localBoxProgressClient struct {
	base interface {
		Do(*http.Request) (*http.Response, error)
	}
	onRead func(int64)
	total  atomic.Int64
}

func (client *localBoxProgressClient) Do(request *http.Request) (*http.Response, error) {
	response, err := client.base.Do(request)
	if err != nil || response == nil || response.Body == nil {
		return response, err
	}
	response.Body = &localBoxProgressBody{ReadCloser: response.Body, client: client}
	return response, nil
}

type localBoxProgressBody struct {
	io.ReadCloser
	client *localBoxProgressClient
}

func (body *localBoxProgressBody) Read(buffer []byte) (int, error) {
	count, err := body.ReadCloser.Read(buffer)
	if count > 0 {
		total := body.client.total.Add(int64(count))
		if body.client.onRead != nil {
			body.client.onRead(total)
		}
	}
	return count, err
}

func dataDirectoryWasEmpty(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("数据目录路径不是目录")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func cleanupNewDataDirectory(path string, wasEmpty bool) error {
	if !wasEmpty {
		return nil
	}
	entries, err := os.ReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return os.Remove(path)
	}
	if len(entries) != 1 || entries[0].Name() != "meta" || !entries[0].IsDir() {
		return nil
	}
	metaEntries, err := os.ReadDir(filepath.Join(path, "meta"))
	if err != nil {
		return err
	}
	if len(metaEntries) != 1 || metaEntries[0].Name() != "local-identity.json" || metaEntries[0].IsDir() {
		return nil
	}
	return os.RemoveAll(path)
}

func localBoxErrorCodeFrom(err error, fallback string) string {
	message := strings.TrimSpace(err.Error())
	for _, code := range []string{
		"LOCAL_BOX_DATA_IN_USE", "LOCAL_BOX_DATA_IDENTITY_MISSING", "LOCAL_BOX_DATA_IDENTITY_MISMATCH",
		"LOCAL_BOX_REGISTRATION_INVALID", "LOCAL_BOX_CREDENTIAL_MISMATCH", "LOCAL_BOX_DOWNLOAD_FAILED",
		"LOCAL_BOX_PACKAGE_INVALID", "LOCAL_BOX_START_FAILED", "LOCAL_BOX_SOURCE_MISMATCH",
		"LOCAL_BOX_DEV_ARTIFACT_UNAVAILABLE", "LOCAL_BOX_DEV_ARTIFACT_INVALID",
	} {
		if strings.Contains(message, code) {
			return code
		}
	}
	return fallback
}
