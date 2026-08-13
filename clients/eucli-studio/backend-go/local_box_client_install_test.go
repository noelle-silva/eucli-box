//go:build windows && eucli_client_install

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/localrun"
	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

type clientInstallCandidateChecker struct {
	candidate *releasecheck.ReleaseCandidate
}

type clientInstallFixture struct {
	manifest     types.ReleaseManifest
	manifestData []byte
	archiveData  []byte
	archiveName  string
	manifestName string
}

func (checker clientInstallCandidateChecker) Kind() localBoxSourceKind {
	return localBoxSourceOfficial
}

func (checker clientInstallCandidateChecker) LatestCandidate(context.Context, types.ReleaseArtifactIdentity) (*releasecheck.ReleaseCandidate, error) {
	return checker.candidate, nil
}

func (checker clientInstallCandidateChecker) AcquireArtifacts(ctx context.Context, candidate *releasecheck.ReleaseCandidate, downloadDir string, onProgress func(localBoxProgress)) (types.ReleaseManifest, error) {
	return acquireOfficialArtifacts(ctx, candidate, downloadDir, onProgress)
}

func TestClientInstallLocalBoxLifecycle(t *testing.T) {
	fixture := loadClientInstallFixture(t)
	clientDataDir := os.Getenv("EUCLI_CLIENT_INSTALL_CLIENT_DATA_DIR")
	if clientDataDir == "" {
		t.Fatal("客户端安装验证缺少隔离成品或数据目录资料")
	}
	server := fixture.server(http.StatusOK, fixture.archiveData)
	defer server.Close()

	paths, err := newLocalBoxPaths(clientDataDir)
	if err != nil {
		t.Fatal(err)
	}

	installState := make([]localBoxState, 0)
	manager := fixture.manager(paths, server.URL, func(state localBoxState) { installState = append(installState, state) })

	state, err := manager.install(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !state.Installed || !state.Connected || state.Status != localBoxStatusConnected {
		t.Fatalf("install state = %#v", state)
	}
	connection := manager.currentConnection()
	if connection == nil {
		t.Fatal("local connection was not established")
	}
	wrongCredentialRequest, err := http.NewRequest(http.MethodGet, connection.BaseURL+"/api/local-run", nil)
	if err != nil {
		t.Fatal(err)
	}
	wrongCredentialRequest.Header.Set("Authorization", "Bearer session-wrong")
	wrongCredentialResponse, err := (&http.Client{}).Do(wrongCredentialRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = wrongCredentialResponse.Body.Close()
	if wrongCredentialResponse.StatusCode == http.StatusOK {
		t.Fatal("wrong credential was accepted")
	}
	correctCredentialRequest, err := http.NewRequest(http.MethodGet, connection.BaseURL+"/api/local-run", nil)
	if err != nil {
		t.Fatal(err)
	}
	correctCredentialRequest.Header.Set("Authorization", "Bearer "+connection.Credential)
	correctCredentialResponse, err := (&http.Client{}).Do(correctCredentialRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = correctCredentialResponse.Body.Close()
	if correctCredentialResponse.StatusCode != http.StatusOK {
		t.Fatalf("correct credential status = %d", correctCredentialResponse.StatusCode)
	}
	lock, err := localrun.AcquireDataLock(paths.dataDir)
	if lock != nil {
		_ = lock.Release()
	}
	if err == nil || !strings.Contains(err.Error(), "LOCAL_BOX_DATA_IN_USE") {
		t.Fatalf("second data lock error = %v, want LOCAL_BOX_DATA_IN_USE", err)
	}
	state, err = manager.stop(context.Background())
	if err != nil || state.Status != localBoxStatusStopped {
		t.Fatalf("stop state=%#v err=%v", state, err)
	}
	if _, err := os.Stat(paths.registrationPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("registration still exists: %v", err)
	}
	if len(installState) < 4 {
		t.Fatalf("state sequence too short: %#v", installState)
	}
}

func TestClientInstallDownloadFailureLeavesNoInstalledState(t *testing.T) {
	fixture := loadClientInstallFixture(t)
	paths := clientInstallTestPaths(t)
	server := fixture.server(http.StatusServiceUnavailable, nil)
	defer server.Close()
	state, err := fixture.manager(paths, server.URL, nil).install(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Error.Code != "LOCAL_BOX_DOWNLOAD_FAILED" || state.Installed {
		t.Fatalf("download failure state = %#v", state)
	}
	assertNoInstallRecord(t, paths)
}

func TestClientInstallDigestMismatchLeavesNoInstalledState(t *testing.T) {
	fixture := loadClientInstallFixture(t)
	paths := clientInstallTestPaths(t)
	tampered := append([]byte(nil), fixture.archiveData...)
	tampered[len(tampered)-1] ^= 0xff
	server := fixture.server(http.StatusOK, tampered)
	defer server.Close()
	state, err := fixture.manager(paths, server.URL, nil).install(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Error.Code != "LOCAL_BOX_PACKAGE_INVALID" || state.Installed {
		t.Fatalf("digest mismatch state = %#v", state)
	}
	assertNoInstallRecord(t, paths)
}

func TestClientInstallInvalidPackageLeavesNoInstalledState(t *testing.T) {
	fixture := loadClientInstallFixture(t)
	paths := clientInstallTestPaths(t)
	invalidArchive := archiveWithoutRequiredFile(t, fixture.archiveData, "README.md")
	invalidManifest := fixture.manifest
	invalidManifest.Archive.Size = int64(len(invalidArchive))
	invalidManifest.Archive.SHA256 = release.SHA256(invalidArchive)
	invalidManifestData, err := json.Marshal(invalidManifest)
	if err != nil {
		t.Fatal(err)
	}
	invalidFixture := fixture
	invalidFixture.manifest = invalidManifest
	invalidFixture.manifestData = invalidManifestData
	invalidFixture.archiveData = invalidArchive
	server := invalidFixture.server(http.StatusOK, invalidArchive)
	defer server.Close()
	state, err := invalidFixture.manager(paths, server.URL, nil).install(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Error.Code != "LOCAL_BOX_PACKAGE_INVALID" || state.Installed {
		t.Fatalf("invalid package state = %#v", state)
	}
	assertNoInstallRecord(t, paths)
}

func TestClientInstallCorruptRegistrationIsRejected(t *testing.T) {
	paths := clientInstallTestPaths(t)
	if err := os.MkdirAll(paths.runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.registrationPath, []byte(`{"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := preparePreviousRegistration(paths.registrationPath); err == nil || !strings.Contains(err.Error(), "LOCAL_BOX_REGISTRATION_INVALID") {
		t.Fatalf("corrupt registration error = %v", err)
	}
}

func loadClientInstallFixture(t *testing.T) clientInstallFixture {
	t.Helper()
	archivePath := os.Getenv("EUCLI_CLIENT_INSTALL_ARCHIVE")
	manifestPath := os.Getenv("EUCLI_CLIENT_INSTALL_MANIFEST")
	if archivePath == "" || manifestPath == "" {
		t.Fatal("客户端安装验证缺少隔离成品资料")
	}
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := release.DecodeReleaseManifest(manifestData)
	if err != nil {
		t.Fatal(err)
	}
	archiveData, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	return clientInstallFixture{manifest: manifest, manifestData: manifestData, archiveData: archiveData, archiveName: filepath.Base(archivePath), manifestName: filepath.Base(manifestPath)}
}

func clientInstallTestPaths(t *testing.T) localBoxPaths {
	t.Helper()
	base := os.Getenv("EUCLI_CLIENT_INSTALL_CLIENT_DATA_DIR")
	if base == "" {
		t.Fatal("客户端安装验证缺少隔离客户端数据目录")
	}
	name := strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(t.Name())
	paths, err := newLocalBoxPaths(filepath.Join(filepath.Dir(base), name))
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func (fixture clientInstallFixture) server(archiveStatus int, archivePayload []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch filepath.Base(request.URL.Path) {
		case fixture.manifestName:
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write(fixture.manifestData)
		case fixture.archiveName:
			writer.WriteHeader(archiveStatus)
			if archiveStatus == http.StatusOK {
				_, _ = writer.Write(archivePayload)
			}
		default:
			http.NotFound(writer, request)
		}
	}))
}

func (fixture clientInstallFixture) manager(paths localBoxPaths, serverURL string, onState func(localBoxState)) *localBoxManager {
	candidate := &releasecheck.ReleaseCandidate{
		Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindBox, ID: types.ReleaseArtifactKindBox},
		Manifest: fixture.manifest, ManifestURL: serverURL + "/" + fixture.manifestName, ManifestSize: int64(len(fixture.manifestData)),
		ArchiveURL: serverURL + "/" + fixture.archiveName,
	}
	return newLocalBoxManager(paths, clientInstallCandidateChecker{candidate: candidate}, onState, nil, nil)
}

func assertNoInstallRecord(t *testing.T, paths localBoxPaths) {
	t.Helper()
	if _, err := os.Stat(paths.installPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("install record exists: %v", err)
	}
}

func archiveWithoutRequiredFile(t *testing.T, payload []byte, excluded string) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	var result bytes.Buffer
	writer := zip.NewWriter(&result)
	for _, file := range reader.File {
		if file.Name == excluded {
			continue
		}
		header := file.FileHeader
		output, err := writer.CreateHeader(&header)
		if err != nil {
			t.Fatal(err)
		}
		input, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(output, input); err != nil {
			_ = input.Close()
			t.Fatal(err)
		}
		if err := input.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return result.Bytes()
}
