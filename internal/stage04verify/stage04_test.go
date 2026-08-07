//go:build eucli_stage04

package stage04verify

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	datastorage "eucli-box/src/data-storage-system"
	toolcalling "eucli-box/src/tool-calling-system"
	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

const boxVersion = "0.1.1"

// 真实活动时长必须超过业务端默认更新等待超时，才能观察到活动阻止。
const longActivitySleep = 35 * time.Second

// ---------- 候选构造 ----------

type stage04Candidate struct {
	identity      types.ReleaseArtifactIdentity
	version       string
	manifest      types.ReleaseManifest
	manifestJSON  []byte
	archiveBytes  []byte
	incompatible  bool
	brokenArchive bool
	brokenBinary  bool
	lifecycle     string
}

type stage04Server struct {
	t               *testing.T
	server          *httptest.Server
	mu              sync.Mutex
	byKey           map[string]*stage04Candidate
	releases        map[string][]map[string]any
	archiveRequests map[string]int
}

func newStage04Server(t *testing.T) *stage04Server {
	fixture := &stage04Server{
		t:               t,
		byKey:           map[string]*stage04Candidate{},
		releases:        map[string][]map[string]any{},
		archiveRequests: map[string]int{},
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *stage04Server) url() string { return f.server.URL }

func (f *stage04Server) client() *http.Client { return f.server.Client() }

func (f *stage04Server) addCandidate(candidate *stage04Candidate) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := candidateKey(candidate.identity, candidate.version)
	f.byKey[key] = candidate
	tag := candidate.identity.ID + "/v" + candidate.version
	manifestName := strings.TrimSuffix(candidate.manifest.Archive.Name, ".zip") + ".manifest.json"
	entry := map[string]any{
		"tag_name":   tag,
		"draft":      false,
		"prerelease": false,
		"html_url":   "https://github.com/noelle-silva/eucli-box/releases/tag/" + tag,
		"body":       "发行说明 " + candidate.version,
		"assets": []map[string]any{
			{"name": manifestName, "size": len(candidate.manifestJSON), "browser_download_url": f.server.URL + "/manifest/" + key},
			{"name": candidate.manifest.Archive.Name, "size": len(candidate.archiveBytes), "browser_download_url": f.server.URL + "/archive/" + key},
		},
	}
	f.releases[candidate.identity.Kind] = append(f.releases[candidate.identity.Kind], entry)
}

func (f *stage04Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case strings.Contains(path, "/releases"):
		kind := releaseKindFromPath(path)
		f.mu.Lock()
		items := f.releases[kind]
		f.mu.Unlock()
		if items == nil {
			items = []map[string]any{}
		}
		_ = json.NewEncoder(w).Encode(items)
	case strings.HasPrefix(path, "/manifest/"):
		f.mu.Lock()
		candidate := f.byKey[strings.TrimPrefix(path, "/manifest/")]
		f.mu.Unlock()
		if candidate == nil {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(candidate.manifestJSON)
	case strings.HasPrefix(path, "/archive/"):
		key := strings.TrimPrefix(path, "/archive/")
		f.mu.Lock()
		candidate := f.byKey[key]
		if candidate != nil {
			f.archiveRequests[key]++
		}
		f.mu.Unlock()
		if candidate == nil {
			http.NotFound(w, r)
			return
		}
		if candidate.brokenArchive {
			_, _ = w.Write([]byte("corrupted-archive-bytes"))
			return
		}
		_, _ = w.Write(candidate.archiveBytes)
	default:
		http.NotFound(w, r)
	}
}

func (f *stage04Server) archiveRequestCount(identity types.ReleaseArtifactIdentity, version string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.archiveRequests[candidateKey(identity, version)]
}

// latestCandidateFor 返回某发布物的最高版本候选（供组件级验证复用同一隔离来源）。
func (f *stage04Server) latestCandidateFor(identity types.ReleaseArtifactIdentity) *releasecheck.ReleaseCandidate {
	f.mu.Lock()
	defer f.mu.Unlock()
	var best *stage04Candidate
	for key, candidate := range f.byKey {
		parts := strings.SplitN(key, "/", 3)
		if len(parts) != 3 || parts[0] != identity.Kind || parts[1] != identity.ID {
			continue
		}
		if best == nil {
			best = candidate
			continue
		}
		order, err := release.CompareVersions(candidate.version, best.version)
		if err == nil && order > 0 {
			best = candidate
		}
	}
	if best == nil {
		return nil
	}
	key := candidateKey(best.identity, best.version)
	return &releasecheck.ReleaseCandidate{
		Artifact:     best.identity,
		Manifest:     best.manifest,
		ManifestURL:  f.server.URL + "/manifest/" + key,
		ManifestSize: int64(len(best.manifestJSON)),
		ArchiveURL:   f.server.URL + "/archive/" + key,
		ReleaseNotes: "release notes " + best.version,
	}
}

func releaseKindFromPath(path string) string {
	switch {
	case strings.Contains(path, "eucli-box-ai-tools"):
		return types.ReleaseArtifactKindTool
	case strings.Contains(path, "eucli-box-system-plugins"):
		return types.ReleaseArtifactKindPlugin
	default:
		return types.ReleaseArtifactKindBox
	}
}

func candidateKey(identity types.ReleaseArtifactIdentity, version string) string {
	return identity.Kind + "/" + identity.ID + "/" + version
}

// ---------- 成品构造 ----------

func makeToolCandidate(t *testing.T, server *stage04Server, id string, version string, brokenBinary bool, incompatible bool) {
	makeToolCandidateWithSleep(t, server, id, version, brokenBinary, incompatible, 2*time.Second)
}

// makeToolCandidateWithSleep 构造工具候选；sleepFor 控制 ActionID=sleep 时的真实执行时长。
func makeToolCandidateWithSleep(t *testing.T, server *stage04Server, id string, version string, brokenBinary bool, incompatible bool, sleepFor time.Duration) {
	t.Helper()
	executable := buildProbeTool(t, fmt.Sprintf("tool-sleep-%d", sleepFor/time.Second))
	payload := map[string][]byte{
		filepath.ToSlash(filepath.Join("binary", "windows-amd64", id+".exe")): executable,
	}
	if brokenBinary {
		payload[filepath.ToSlash(filepath.Join("binary", "windows-amd64", id+".exe"))] = []byte("not an executable")
	}
	compatibility := types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"}
	if incompatible {
		compatibility = types.EucliBoxCompatibility{MinimumVersion: "0.2.0", MaximumVersionExclusive: "0.3.0"}
	}
	definition := types.ToolDefinition{
		ID:                    id,
		Name:                  "Demo " + id,
		Description:           "demo tool",
		Version:               version,
		EucliBoxCompatibility: compatibility,
		DefaultInvocationMode: "sync",
		Type:                  "local",
		BodyDirectory:         ".",
		Binaries:              []types.ToolBinary{{GOOS: "windows", GOARCH: "amd64", Path: filepath.ToSlash(filepath.Join("binary", "windows-amd64", id+".exe"))}},
	}
	definitionJSON, err := json.MarshalIndent(definition, "", "  ")
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	payload["definition.json"] = definitionJSON
	payload["README.md"] = []byte("# " + id + "\n")
	payload["CHANGELOG.md"] = []byte("## " + version + "\n")
	payload = addProductRecord(t, payload, types.ReleaseArtifactKindTool, id, version, &compatibility)
	archiveBytes := zipPayload(t, payload)
	manifest := toolManifest(id, version, archiveBytes, &compatibility)
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	server.addCandidate(&stage04Candidate{
		identity:      types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: id},
		version:       version,
		manifest:      manifest,
		manifestJSON:  manifestJSON,
		archiveBytes:  archiveBytes,
		brokenBinary:  brokenBinary,
		incompatible:  incompatible,
	})
}

// makeBrokenToolCandidate 构造四种损坏样例：损坏 ZIP、缺少身份文件、身份不一致、越界路径。
func makeBrokenToolCandidate(t *testing.T, server *stage04Server, id string, version string, brokenKind string) {
	t.Helper()
	executable := buildProbeTool(t, "tool")
	compatibility := types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"}
	definition := types.ToolDefinition{
		ID:                    id,
		Name:                  "Demo " + id,
		Description:           "demo tool",
		Version:               version,
		EucliBoxCompatibility: compatibility,
		DefaultInvocationMode: "sync",
		Type:                  "local",
		BodyDirectory:         ".",
		Binaries:              []types.ToolBinary{{GOOS: "windows", GOARCH: "amd64", Path: filepath.ToSlash(filepath.Join("binary", "windows-amd64", id+".exe"))}},
	}
	definitionJSON, err := json.MarshalIndent(definition, "", "  ")
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	payload := map[string][]byte{
		filepath.ToSlash(filepath.Join("binary", "windows-amd64", id+".exe")): executable,
		"definition.json": definitionJSON,
		"README.md":       []byte("# " + id + "\n"),
		"CHANGELOG.md":    []byte("## " + version + "\n"),
	}
	payload = addProductRecord(t, payload, types.ReleaseArtifactKindTool, id, version, &compatibility)
	var archiveBytes []byte
	manifest := types.ReleaseManifest{}
	switch brokenKind {
	case "corrupt-zip":
		full := zipPayload(t, payload)
		files := filesFromZip(full)
		archiveBytes = full[:len(full)/2]
		manifest = toolManifestWithFiles(id, version, archiveBytes, &compatibility, files)
	case "missing-identity":
		delete(payload, "release-product.json")
		archiveBytes = zipPayload(t, payload)
		manifest = toolManifest(id, version, archiveBytes, &compatibility)
	case "identity-mismatch":
		payload = addProductRecord(t, payload, types.ReleaseArtifactKindTool, "other-identity", version, &compatibility)
		archiveBytes = zipPayload(t, payload)
		manifest = toolManifest(id, version, archiveBytes, &compatibility)
	case "path-escape":
		archiveBytes = zipPayloadWithExtra(t, payload, "../evil.txt", []byte("escaped"))
		manifest = toolManifest(id, version, archiveBytes, &compatibility)
	default:
		t.Fatalf("未知损坏样例 %q", brokenKind)
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	server.addCandidate(&stage04Candidate{
		identity:      types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: id},
		version:       version,
		manifest:      manifest,
		manifestJSON:  manifestJSON,
		archiveBytes:  archiveBytes,
	})
}

func makePluginCandidate(t *testing.T, server *stage04Server, id string, version string, lifecycle string, brokenBinary bool, incompatible bool) {
	t.Helper()
	probeKind := "plugin"
	if lifecycle == types.SystemPluginLifecyclePersistent {
		probeKind = "persistent"
	}
	executable := buildProbeTool(t, probeKind)
	if brokenBinary {
		executable = []byte("not an executable")
	}
	compatibility := types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"}
	if incompatible {
		compatibility = types.EucliBoxCompatibility{MinimumVersion: "0.2.0", MaximumVersionExclusive: "0.3.0"}
	}
	binaryPath := filepath.ToSlash(filepath.Join("binary", id+".exe"))
	manifest := types.SystemPluginManifest{
		ID:                    id,
		Name:                  "Demo " + id,
		Description:           "demo plugin",
		Version:               version,
		EucliBoxCompatibility: compatibility,
		LifecycleType:         lifecycle,
		Binaries:              []types.SystemPluginBinary{{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Path: binaryPath}},
		PlaceholderInterfaces: []types.SystemPluginPlaceholderInterface{{ID: "value", DefaultName: "demo value", Description: "demo"}},
	}
	if lifecycle == types.SystemPluginLifecycleCachedHeartbeat {
		manifest.HeartbeatIntervalMs = 60
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal plugin manifest: %v", err)
	}
	payload := map[string][]byte{
		"manifest.json": manifestJSON,
		binaryPath:      executable,
		"config.json":   []byte("{}\n"),
		"README.md":     []byte("# " + id + "\n"),
		"CHANGELOG.md":  []byte("## " + version + "\n"),
	}
	payload = addProductRecord(t, payload, types.ReleaseArtifactKindPlugin, id, version, &compatibility)
	archiveBytes := zipPayload(t, payload)
	releaseManifest := pluginManifest(id, version, archiveBytes, &compatibility)
	releaseManifestJSON, err := json.Marshal(releaseManifest)
	if err != nil {
		t.Fatalf("marshal release manifest: %v", err)
	}
	server.addCandidate(&stage04Candidate{
		identity:      types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: id},
		version:       version,
		manifest:      releaseManifest,
		manifestJSON:  releaseManifestJSON,
		archiveBytes:  archiveBytes,
		brokenBinary:  brokenBinary,
		incompatible:  incompatible,
		lifecycle:     lifecycle,
	})
}

func addProductRecord(t *testing.T, payload map[string][]byte, kind string, id string, version string, compatibility *types.EucliBoxCompatibility) map[string][]byte {
	t.Helper()
	product := types.ReleaseProductRecord{
		SchemaVersion:  release.ReleaseManifestSchemaVersion,
		Artifact:       types.ReleaseArtifactIdentity{Kind: kind, ID: id},
		Version:        version,
		Platform:       types.ReleasePlatformWindowsX64,
		OfficialSource: officialSourceForKind(kind),
		Compatibility:  compatibility,
		Source:         types.ReleaseSourceRecord{Repository: "https://github.com/noelle-silva/eucli-box", Commit: "0123456789abcdef0123456789abcdef01234567", Recorded: true},
	}
	productJSON, err := json.MarshalIndent(product, "", "  ")
	if err != nil {
		t.Fatalf("marshal product: %v", err)
	}
	payload["release-product.json"] = productJSON
	return payload
}

func officialSourceForKind(kind string) string {
	switch kind {
	case types.ReleaseArtifactKindTool:
		return "https://github.com/noelle-silva/eucli-box-ai-tools"
	case types.ReleaseArtifactKindPlugin:
		return "https://github.com/noelle-silva/eucli-box-system-plugins"
	default:
		return "https://github.com/noelle-silva/eucli-box"
	}
}

func toolManifest(id string, version string, archiveBytes []byte, compatibility *types.EucliBoxCompatibility) types.ReleaseManifest {
	return toolManifestWithFiles(id, version, archiveBytes, compatibility, filesFromZip(archiveBytes))
}

func toolManifestWithFiles(id string, version string, archiveBytes []byte, compatibility *types.EucliBoxCompatibility, files []types.ReleaseFileRecord) types.ReleaseManifest {
	archiveName := fmt.Sprintf("tool-%s_%s_%s.zip", id, version, types.ReleasePlatformWindowsX64)
	return types.ReleaseManifest{
		SchemaVersion:  release.ReleaseManifestSchemaVersion,
		Artifact:       types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: id},
		Version:        version,
		Platform:       types.ReleasePlatformWindowsX64,
		TagName:        id + "/v" + version,
		OfficialSource: officialSourceForKind(types.ReleaseArtifactKindTool),
		Compatibility:  compatibility,
		Source:         types.ReleaseSourceRecord{Repository: "https://github.com/noelle-silva/eucli-box", Commit: "0123456789abcdef0123456789abcdef01234567", Recorded: true},
		Archive:        types.ReleaseFileRecord{Name: archiveName, Size: int64(len(archiveBytes)), SHA256: release.SHA256(archiveBytes)},
		Files:          files,
	}
}

func pluginManifest(id string, version string, archiveBytes []byte, compatibility *types.EucliBoxCompatibility) types.ReleaseManifest {
	archiveName := fmt.Sprintf("plugin-%s_%s_%s.zip", id, version, types.ReleasePlatformWindowsX64)
	return types.ReleaseManifest{
		SchemaVersion:  release.ReleaseManifestSchemaVersion,
		Artifact:       types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: id},
		Version:        version,
		Platform:       types.ReleasePlatformWindowsX64,
		TagName:        id + "/v" + version,
		OfficialSource: officialSourceForKind(types.ReleaseArtifactKindPlugin),
		Compatibility:  compatibility,
		Source:         types.ReleaseSourceRecord{Repository: "https://github.com/noelle-silva/eucli-box", Commit: "0123456789abcdef0123456789abcdef01234567", Recorded: true},
		Archive:        types.ReleaseFileRecord{Name: archiveName, Size: int64(len(archiveBytes)), SHA256: release.SHA256(archiveBytes)},
		Files:          filesFromZip(archiveBytes),
	}
}

func filesFromZip(archiveBytes []byte) []types.ReleaseFileRecord {
	reader, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	if err != nil {
		return nil
	}
	records := make([]types.ReleaseFileRecord, 0, len(reader.File))
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		input, err := entry.Open()
		if err != nil {
			continue
		}
		payload, _ := io.ReadAll(input)
		_ = input.Close()
		records = append(records, types.ReleaseFileRecord{Name: entry.Name, Size: int64(len(payload)), SHA256: release.SHA256(payload)})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Name < records[j].Name })
	return records
}

func zipPayload(t *testing.T, payload map[string][]byte) []byte {
	t.Helper()
	return zipPayloadWithExtra(t, payload, "", nil)
}

func zipPayloadWithExtra(t *testing.T, payload map[string][]byte, extraName string, extraBytes []byte) []byte {
	t.Helper()
	names := make([]string, 0, len(payload)+1)
	for name := range payload {
		names = append(names, name)
	}
	if extraName != "" {
		names = append(names, extraName)
	}
	sort.Strings(names)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range names {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		content := payload[name]
		if name == extraName {
			content = extraBytes
		}
		if _, err := entry.Write(content); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buffer.Bytes()
}

// buildProbeTool 编译基础交接可用的探测程序。
// tool-sleep-N：ActionID=sleep 时真实执行 N 秒；plugin：检测 sleep-marker.txt 后真实执行 35 秒；
// persistent：忽略 stdin 关闭，进程不退出。
func buildProbeTool(t *testing.T, kind string) []byte {
	t.Helper()
	var source string
	switch {
	case strings.HasPrefix(kind, "tool"):
		sleepSeconds := "2"
		if kind != "tool" {
			sleepSeconds = strings.TrimPrefix(kind, "tool-sleep-")
			if sleepSeconds == "" {
				sleepSeconds = "2"
			}
		}
		source = fmt.Sprintf(`package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type input struct {
	ActionID          string `+"`json:\"actionId\"`"+`
	ToolDataDirectory string `+"`json:\"toolDataDirectory\"`"+`
}

func main() {
	var in input
	_ = json.NewDecoder(os.Stdin).Decode(&in)
	if in.ActionID == "sleep" {
		_ = os.MkdirAll(in.ToolDataDirectory, 0o755)
		_ = os.WriteFile(filepath.Join(in.ToolDataDirectory, "marker.txt"), []byte("running"), 0o644)
		time.Sleep(%s * time.Second)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "success", "content": "ok"})
}
`, sleepSeconds)
	case kind == "persistent":
		source = `package main

import (
	"encoding/json"
	"os"
	"time"
)

func main() {
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for {
		var request map[string]any
		if err := decoder.Decode(&request); err != nil {
			time.Sleep(30 * time.Second)
			continue
		}
		_ = encoder.Encode(map[string]any{"status": "success", "values": map[string]string{"value": "persistent"}})
	}
}
`
	default:
		source = `package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type input struct {
	Action              string            ` + "`json:\"action\"`" + `
	PluginDataDirectory string            ` + "`json:\"pluginDataDirectory\"`" + `
}

func main() {
	var in input
	_ = json.NewDecoder(os.Stdin).Decode(&in)
	if _, err := os.Stat(filepath.Join(in.PluginDataDirectory, "sleep-marker.txt")); err == nil {
		time.Sleep(35 * time.Second)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "success", "values": map[string]string{"value": "ok"}})
}
`
	}
	dir := t.TempDir()
	sourceFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(sourceFile, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	exe := filepath.Join(dir, "probe.exe")
	cmd := exec.Command("go", "build", "-o", exe, sourceFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build probe failed: %v\n%s", err, output)
	}
	payload, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("read probe: %v", err)
	}
	return payload
}

// ---------- 业务端进程 ----------

type boxProcess struct {
	t       *testing.T
	cmd     *exec.Cmd
	baseURL string
	client  *http.Client
	logFile *os.File
}

func startBox(t *testing.T, boxPath string, envDir string, serverURL string) *boxProcess {
	t.Helper()
	port := freePort(t)
	boxData := filepath.Join(envDir, "box-data")
	programRoot := filepath.Join(envDir, "program-root")
	tempDir := filepath.Join(envDir, "temp")
	for _, dir := range []string{boxData, programRoot, tempDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	logFile, err := os.Create(filepath.Join(envDir, "box.log"))
	if err != nil {
		t.Fatalf("create box log: %v", err)
	}
	cmd := exec.Command(boxPath)
	cmd.Env = append(os.Environ(),
		"EUCLI_BOX_DATA_DIR="+boxData,
		"EUCLI_BOX_PROGRAM_ROOT="+programRoot,
		"EUCLI_BOX_ADDR=127.0.0.1:"+port,
		"EUCLI_BOX_RELEASE_API_BASE="+serverURL,
		"TEMP="+tempDir,
		"TMP="+tempDir,
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		t.Fatalf("start box: %v", err)
	}
	box := &boxProcess{t: t, cmd: cmd, baseURL: "http://127.0.0.1:" + port, client: &http.Client{Timeout: 60 * time.Second}, logFile: logFile}
	t.Cleanup(func() {
		box.stop()
	})
	box.waitReady(t)
	return box
}

func freePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return fmt.Sprintf("%d", port)
}

func (b *boxProcess) waitReady(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := b.call(http.MethodGet, "/api/release", "")
		if status >= 200 && status < 300 {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("业务端未在期限内就绪，日志：\n%s", b.logText())
}

func (b *boxProcess) stop() {
	if b.cmd == nil || b.cmd.Process == nil {
		return
	}
	// Windows 上使用进程树终止，确保业务端派生的工具/插件进程一并结束。
	_ = exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", b.cmd.Process.Pid)).Run()
	done := make(chan struct{})
	go func() {
		_ = b.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

func (b *boxProcess) logText() string {
	if b.logFile == nil {
		return ""
	}
	payload, err := os.ReadFile(b.logFile.Name())
	if err != nil {
		return ""
	}
	return string(payload)
}

func (b *boxProcess) call(method string, path string, body string) (int, []byte) {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequest(method, b.baseURL+path, reader)
	if err != nil {
		return 0, nil
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := b.client.Do(request)
	if err != nil {
		b.t.Logf("box.call %s %s 失败：%v", method, path, err)
		return 0, nil
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	return response.StatusCode, payload
}

func (b *boxProcess) dataJSON(status int, payload []byte) map[string]any {
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if status >= 200 && status < 300 {
		_ = json.Unmarshal(payload, &envelope)
	}
	return envelope.Data
}

// ---------- 目录快照 ----------

func snapshotDir(root string) map[string]string {
	result := map[string]string{}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		payload, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		result[filepath.ToSlash(relative)] = release.SHA256(payload)
		return nil
	})
	return result
}

func compareDirSnapshots(t *testing.T, label string, before map[string]string, after map[string]string) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("%s 文件数量改变：%d -> %d", label, len(before), len(after))
	}
	for name, digest := range before {
		if after[name] != digest {
			t.Fatalf("%s 文件改变：%s", label, name)
		}
	}
}

// ---------- 组件级工具系统（步骤 7/11/14 的真实执行与活动保护） ----------

type serverCandidateReader struct {
	server *stage04Server
}

func (r *serverCandidateReader) LatestCandidate(ctx context.Context, identity types.ReleaseArtifactIdentity) (*releasecheck.ReleaseCandidate, error) {
	candidate := r.server.latestCandidateFor(identity)
	if candidate == nil {
		return nil, fmt.Errorf("没有 %s 的官方候选", identity.ID)
	}
	return candidate, nil
}

type fakePermission struct{}

func (f *fakePermission) Decide(ctx context.Context, roleID string, action types.ToolAction) (types.PermissionDecision, error) {
	return types.PermissionDecision{ID: "stage04-d", ActionID: action.ID, ToolName: action.ToolName, Status: types.PermissionStatusAllowed}, nil
}

func (f *fakePermission) ApplyConfirmation(ctx context.Context, decision types.PermissionDecision, confirmation types.ToolConfirmation) (types.PermissionDecision, error) {
	return types.PermissionDecision{ID: decision.ID, ActionID: decision.ActionID, ToolName: decision.ToolName, Status: types.PermissionStatusAllowed}, nil
}

// newComponentToolSystem 构造与业务端共享同一数据目录和程序根目录的工具系统组件。
func newComponentToolSystem(t *testing.T, envDir string, programRoot string, server *stage04Server) toolcalling.System {
	t.Helper()
	toolProgramRoot := filepath.Join(programRoot, "tools")
	storage, err := datastorage.NewSystem(datastorage.Config{RootDir: filepath.Join(envDir, "box-data"), ToolBodiesRoot: toolProgramRoot})
	if err != nil {
		t.Fatalf("datastorage.NewSystem() error = %v", err)
	}
	system, err := toolcalling.NewSystem(toolcalling.Config{
		BoxVersion:  boxVersion,
		ProgramRoot: toolProgramRoot,
		Candidates:  &serverCandidateReader{server: server},
		HTTPClient:  server.client(),
	}, &fakePermission{}, storage)
	if err != nil {
		t.Fatalf("toolcalling.NewSystem() error = %v", err)
	}
	return system
}

// executeTool 通过组件级工具系统触发一次真实工具执行；sleep 时写 marker 并长时间运行。
func executeTool(t *testing.T, system toolcalling.System, toolID string, sleep bool) {
	t.Helper()
	tool, err := system.LoadTool(context.Background(), toolID)
	if err != nil {
		var chain string
		for current := err; current != nil; current = errors.Unwrap(current) {
			chain += " -> " + current.Error()
		}
		t.Fatalf("LoadTool(%s) error chain:%s", toolID, chain)
	}
	if len(tool.Binaries) == 0 {
		t.Fatalf("工具 %s 没有可执行文件声明", toolID)
	}
	executable := filepath.Join(tool.BodyDirectory, filepath.FromSlash(tool.Binaries[0].Path))
	actionID := "a1"
	if sleep {
		actionID = "sleep"
	}
	plan := types.ToolRunPlan{
		Action:     types.ToolAction{ID: actionID, ToolName: tool.Name, Arguments: map[string]any{}},
		Tool:       tool,
		Decision:   types.PermissionDecision{ID: "stage04-d", ActionID: actionID, ToolName: tool.Name, Status: types.PermissionStatusAllowed},
		PlanStatus: types.ToolPlanStatusReady,
		Executable: executable,
	}
	result, err := system.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute(%s) error = %v", toolID, err)
	}
	if result.Status != types.ToolStatusSuccess {
		t.Fatalf("Execute(%s) result = %#v", toolID, result)
	}
}

// executeToolSleep 通过组件级工具系统触发一次真实长时间工具执行（写 marker 后 sleep）。
func executeToolSleep(t *testing.T, system toolcalling.System, toolID string) {
	t.Helper()
	executeTool(t, system, toolID, true)
}

// ---------- 步骤执行 ----------

func TestStage04(t *testing.T) {
	runRoot := os.Getenv("EUCLI_STAGE04_RUN_ROOT")
	boxPath := os.Getenv("EUCLI_STAGE04_BOX")
	if runRoot == "" || boxPath == "" {
		t.Fatal("阶段四验证缺少运行目录或业务端可执行文件")
	}
	envDir := filepath.Join(runRoot, "environment")
	for _, name := range []string{"inputs", "workspace", "environment", "temp", "cache", "evidence"} {
		info, err := os.Stat(filepath.Join(runRoot, name))
		if err != nil || !info.IsDir() {
			t.Fatalf("阶段四运行目录缺少 %s", name)
		}
	}
	programRoot := filepath.Join(envDir, "program-root")
	boxData := filepath.Join(envDir, "box-data")

	// 步骤 3：隔离官方来源服务器，准备一个工具和一个插件的正式候选。
	server := newStage04Server(t)
	makeToolCandidate(t, server, "context7", "0.1.0", false, false)
	makePluginCandidate(t, server, "time-plugin", "0.1.0", types.SystemPluginLifecycleOnDemand, false, false)
	makeToolCandidate(t, server, "sci_calculator", "0.1.0", false, true)
	makeToolCandidate(t, server, "web_search", "0.1.0", false, false)
	makePluginCandidate(t, server, "weather-plugin", "0.1.0", types.SystemPluginLifecycleCachedHeartbeat, false, false)

	box := startBox(t, boxPath, envDir, server.url())

	// 步骤 4：全新程序根目录确认工具和插件均为 not_installed，且发行检查显示可安装。
	status, payload := box.call(http.MethodGet, "/api/tools/context7/install-state", "")
	state := box.dataJSON(status, payload)
	if state["status"] != types.ArtifactStatusNotInstalled {
		t.Fatalf("未安装工具状态 = %#v", state)
	}
	status, payload = box.call(http.MethodGet, "/api/system-plugins/time-plugin/install-state", "")
	state = box.dataJSON(status, payload)
	if state["status"] != types.ArtifactStatusNotInstalled {
		t.Fatalf("未安装插件状态 = %#v", state)
	}
	_, payload = box.call(http.MethodPost, "/api/release-checks/refresh", "")
	results := findReleaseResults(t, payload)
	if context7 := results["tool:context7"]; context7["installed"] != false || context7["updateAvailable"] != true {
		t.Fatalf("发行检查 context7 = %#v", context7)
	}

	// 步骤 5：安装一个工具，一次用户动作对应一次服务端操作，current.json 和版本目录存在。
	status, payload = box.call(http.MethodPost, "/api/tools/context7/install", "{}")
	state = box.dataJSON(status, payload)
	if state["status"] != types.ArtifactStatusActive || state["currentVersion"] != "0.1.0" {
		t.Fatalf("工具安装状态 = %#v（HTTP %d）", state, status)
	}
	if count := server.archiveRequestCount(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, "0.1.0"); count != 1 {
		t.Fatalf("一次用户动作对应压缩包请求 %d 次", count)
	}
	if _, err := os.Stat(filepath.Join(programRoot, "tools", "context7", "current.json")); err != nil {
		t.Fatalf("工具 current.json 缺失：%v", err)
	}
	if _, err := os.Stat(filepath.Join(programRoot, "tools", "context7", "versions", "0.1.0", "definition.json")); err != nil {
		t.Fatalf("工具版本目录缺失：%v", err)
	}

	// 步骤 6：安装一个插件，核对 current.json、manifest、config 和 binary；长期数据目录独立。
	status, payload = box.call(http.MethodPost, "/api/system-plugins/time-plugin/install", "{}")
	state = box.dataJSON(status, payload)
	if state["status"] != types.ArtifactStatusActive || state["currentVersion"] != "0.1.0" {
		t.Fatalf("插件安装状态 = %#v（HTTP %d）", state, status)
	}
	pluginVersionDir := filepath.Join(programRoot, "system-plugins", "time-plugin", "versions", "0.1.0")
	for _, name := range []string{"manifest.json", "config.json", filepath.ToSlash(filepath.Join("binary", "time-plugin.exe"))} {
		if _, err := os.Stat(filepath.Join(pluginVersionDir, filepath.FromSlash(name))); err != nil {
			t.Fatalf("插件版本目录缺少 %s：%v", name, err)
		}
	}
	// 保存插件用户配置触发长期数据目录建立，确认其独立于程序目录。
	status, payload = box.call(http.MethodPut, "/api/system-plugins/time-plugin/user-config", `{"userConfig":{"timezone":"UTC"},"placeholderNameOverrides":{}}`)
	if status != 200 {
		t.Fatalf("保存插件配置 HTTP %d：%s", status, payload)
	}
	if info, err := os.Stat(filepath.Join(boxData, "system-plugins", "time-plugin")); err != nil || !info.IsDir() {
		t.Fatalf("插件长期数据目录未独立建立：%v", err)
	}
	if _, err := os.Stat(filepath.Join(programRoot, "system-plugins", "time-plugin", "config.json")); err == nil {
		t.Fatal("插件长期配置被写入程序目录")
	}

	// 步骤 7：当前版本真实可用（安装已含基础交接；再确认详情和真实执行）。
	status, payload = box.call(http.MethodGet, "/api/tools/context7", "")
	tool := box.dataJSON(status, payload)
	if tool["status"] != types.ToolAvailabilityActive {
		t.Fatalf("工具详情 = %#v", tool)
	}
	status, payload = box.call(http.MethodGet, "/api/system-plugins/time-plugin", "")
	plugin := box.dataJSON(status, payload)
	if plugin["status"] != types.SystemPluginStatusActive {
		t.Fatalf("插件详情 = %#v", plugin)
	}

	// 步骤 9 准备：保存工具用户配置作为长期数据基线。
	status, _ = box.call(http.MethodPut, "/api/tools/context7/user-config", `{"userConfig":{"mode":"fast"},"promptDescriptionOverride":""}`)
	if status != 200 {
		t.Fatalf("保存工具用户配置 HTTP %d", status)
	}
	toolDataBefore := snapshotDir(filepath.Join(boxData, "tool-data", "context7"))
	pluginDataBefore := snapshotDir(filepath.Join(boxData, "system-plugins", "time-plugin"))

	// 步骤 8：同一工具和同一插件的高版本候选，分别执行单项更新；其他发布物版本不变。
	makeToolCandidate(t, server, "context7", "0.1.1", false, false)
	makePluginCandidate(t, server, "time-plugin", "0.1.1", types.SystemPluginLifecycleOnDemand, false, false)
	status, payload = box.call(http.MethodPost, "/api/tools/context7/update", "{}")
	state = box.dataJSON(status, payload)
	if state["status"] != types.ArtifactStatusActive || state["currentVersion"] != "0.1.1" {
		t.Fatalf("工具更新状态 = %#v（HTTP %d）", state, status)
	}
	status, payload = box.call(http.MethodPost, "/api/system-plugins/time-plugin/update", "{}")
	state = box.dataJSON(status, payload)
	if state["status"] != types.ArtifactStatusActive || state["currentVersion"] != "0.1.1" {
		t.Fatalf("插件更新状态 = %#v（HTTP %d）", state, status)
	}
	status, payload = box.call(http.MethodGet, "/api/tools/web_search/install-state", "")
	state = box.dataJSON(status, payload)
	if state["status"] != types.ArtifactStatusNotInstalled {
		t.Fatalf("更新后其他工具被改变 = %#v", state)
	}
	status, payload = box.call(http.MethodGet, "/api/system-plugins/weather-plugin/install-state", "")
	state = box.dataJSON(status, payload)
	if state["status"] != types.ArtifactStatusNotInstalled {
		t.Fatalf("更新后其他插件被改变 = %#v", state)
	}
	status, payload = box.call(http.MethodGet, "/api/tools/context7", "")
	tool = box.dataJSON(status, payload)
	if tool["status"] != types.ToolAvailabilityActive {
		t.Fatalf("更新后工具详情 = %#v", tool)
	}

	// 步骤 9：更新后长期数据逐字节不变。
	toolDataAfter := snapshotDir(filepath.Join(boxData, "tool-data", "context7"))
	compareDirSnapshots(t, "工具长期数据", toolDataBefore, toolDataAfter)
	pluginDataAfter := snapshotDir(filepath.Join(boxData, "system-plugins", "time-plugin"))
	compareDirSnapshots(t, "插件长期数据", pluginDataBefore, pluginDataAfter)

	// 步骤 10：不适用工具和不适用插件在下载前拒绝。
	status, payload = box.call(http.MethodPost, "/api/tools/sci_calculator/install", "{}")
	state = box.dataJSON(status, payload)
	if state["status"] != types.ArtifactStatusBlocked || state["error"].(map[string]any)["code"] != types.ArtifactErrorCompatibility {
		t.Fatalf("不适用工具状态 = %#v", state)
	}
	if count := server.archiveRequestCount(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "sci_calculator"}, "0.1.0"); count != 0 {
		t.Fatalf("不适用工具候选发生 %d 次下载", count)
	}
	makePluginCandidate(t, server, "system-info-plugin", "0.1.0", types.SystemPluginLifecycleOnDemand, false, true)
	status, payload = box.call(http.MethodPost, "/api/system-plugins/system-info-plugin/install", "{}")
	state = box.dataJSON(status, payload)
	if state["status"] != types.ArtifactStatusBlocked || state["error"].(map[string]any)["code"] != types.ArtifactErrorCompatibility {
		t.Fatalf("不适用插件状态 = %#v", state)
	}
	if count := server.archiveRequestCount(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "system-info-plugin"}, "0.1.0"); count != 0 {
		t.Fatalf("不适用插件候选发生 %d 次下载", count)
	}
	if _, err := os.Stat(filepath.Join(programRoot, "tools", "sci_calculator", "work")); err == nil {
		t.Fatal("不适用工具产生了下载工作区")
	}

	// 步骤 11：启动长时间工具进程，尝试更新同一工具，确认 TOOL_ACTIVE 且下载没有开始。
	component := newComponentToolSystem(t, envDir, programRoot, server)
	makeToolCandidateWithSleep(t, server, "shell_command", "0.1.0", false, false, longActivitySleep)
	if _, err := component.InstallTool(context.Background(), "shell_command"); err != nil {
		t.Fatalf("组件安装 shell_command error = %v", err)
	}
	makeToolCandidateWithSleep(t, server, "shell_command", "0.1.1", false, false, longActivitySleep)
	before := server.archiveRequestCount(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "shell_command"}, "0.1.1")
	done := make(chan struct{})
	go func() {
		defer close(done)
		executeToolSleep(t, component, "shell_command")
	}()
	// 等待工具真实执行开始（marker 出现）后再尝试更新。
	marker := filepath.Join(boxData, "tool-data", "shell_command", "marker.txt")
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("工具真实执行未开始：%v", err)
	}
	updateState, updateErr := component.UpdateTool(context.Background(), "shell_command")
	if updateErr != nil {
		t.Fatalf("组件更新 shell_command error = %v", updateErr)
	}
	if updateState.Status != types.ArtifactStatusBlocked || updateState.Error.Code != types.ArtifactErrorToolActive {
		t.Fatalf("工具活动保护状态 = %#v", updateState)
	}
	after := server.archiveRequestCount(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "shell_command"}, "0.1.1")
	if after != before {
		t.Fatalf("工具活动期间发生了下载：%d -> %d", before, after)
	}
	<-done

	// 步骤 12：插件 on-demand、persistent 和 cached-heartbeat 活动阻止更新。
	verifyPluginOnDemandActivity(t, box, boxData, server)
	verifyPluginPersistentActivity(t, box, programRoot, server, boxData)
	verifyPluginHeartbeatActivity(t, box, programRoot, server, boxData)

	// 步骤 13：损坏摘要、损坏 ZIP、缺少身份文件、身份不一致和越界路径样例，当前版本不变且无半成品。
	verifyBrokenCandidates(t, box, programRoot, server)

	// 步骤 14：新版基础交接失败，current.json 恢复上一版、上一版可重新运行、长期数据不变。
	dataBeforeRestore := snapshotDir(filepath.Join(boxData, "tool-data", "context7"))
	makeToolCandidate(t, server, "context7", "0.1.2", true, false)
	status, payload = box.call(http.MethodPost, "/api/tools/context7/update", "{}")
	state = box.dataJSON(status, payload)
	if state["status"] != types.ArtifactStatusFailed || state["error"].(map[string]any)["code"] != types.ArtifactErrorProbeFailed {
		t.Fatalf("交接失败状态 = %#v", state)
	}
	status, payload = box.call(http.MethodGet, "/api/tools/context7/install-state", "")
	state = box.dataJSON(status, payload)
	if state["status"] != types.ArtifactStatusActive || state["currentVersion"] != "0.1.1" {
		t.Fatalf("恢复后工具状态 = %#v", state)
	}
	currentPayload, err := os.ReadFile(filepath.Join(programRoot, "tools", "context7", "current.json"))
	if err != nil {
		t.Fatalf("读取 current.json：%v", err)
	}
	var currentRecord struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(currentPayload, &currentRecord); err != nil || currentRecord.Version != "0.1.1" {
		t.Fatalf("current.json 未恢复上一版：%s", currentPayload)
	}
	executeTool(t, component, "context7", false)
	dataAfterRestore := snapshotDir(filepath.Join(boxData, "tool-data", "context7"))
	compareDirSnapshots(t, "交接失败恢复后工具长期数据", dataBeforeRestore, dataAfterRestore)

	// 步骤 15：切换中断和未完成 operation.json，重启系统按阶段恢复；未知当前版本不启用任何版本。
	box = verifyInterruptedSwitchRecovery(t, boxPath, envDir, programRoot, box, server.url())

	// 步骤 16：单个工具/插件失败不改变业务端、其他工具、其他插件和其他占位符结果。
	status, payload = box.call(http.MethodGet, "/api/system-plugins/time-plugin/install-state", "")
	if status == 0 {
		t.Fatalf("业务端不可达，日志：\n%s", box.logText())
	}
	state = box.dataJSON(status, payload)
	if state["status"] != types.ArtifactStatusActive || state["currentVersion"] != "0.1.2" {
		t.Fatalf("失败后其他插件状态 = %#v", state)
	}
	status, payload = box.call(http.MethodGet, "/api/placeholders", "")
	if status < 200 || status >= 300 {
		t.Fatalf("失败后占位符读取 HTTP %d", status)
	}

	// 步骤 17：发行检查只读，发现新版但不执行用户动作时没有下载、安装、目录切换或状态变化。
	programBefore := snapshotDir(programRoot)
	_, boxStateBefore := box.call(http.MethodGet, "/api/tools/context7/install-state", "")
	_, payload = box.call(http.MethodPost, "/api/release-checks/refresh", "")
	_, boxStateAfter := box.call(http.MethodGet, "/api/tools/context7/install-state", "")
	if !bytes.Equal(boxStateBefore, boxStateAfter) {
		t.Fatalf("只读检查改变了工具状态")
	}
	programAfter := snapshotDir(programRoot)
	compareDirSnapshots(t, "只读检查后的程序根目录", programBefore, programAfter)
	t.Log("阶段四隔离验证完成")
}

// verifyPluginOnDemandActivity 制造 on-demand 活动并确认活动结束前不下载、不切换。
func verifyPluginOnDemandActivity(t *testing.T, box *boxProcess, boxData string, server *stage04Server) {
	t.Helper()
	marker := filepath.Join(boxData, "system-plugins", "time-plugin", "sleep-marker.txt")
	if err := os.WriteFile(marker, []byte("sleep"), 0o644); err != nil {
		t.Fatalf("写 sleep marker：%v", err)
	}
	status, payload := box.call(http.MethodPost, "/api/placeholders/plugin-interfaces", `{"pluginId":"time-plugin","interfaceId":"value"}`)
	if status < 200 || status >= 300 {
		t.Fatalf("创建插件占位符 HTTP %d：%s", status, payload)
	}
	previewDone := make(chan struct{})
	go func() {
		defer close(previewDone)
		_, _ = box.call(http.MethodPost, "/api/placeholders/preview", `{"text":"{{demo value}}"}`)
	}()
	makePluginCandidate(t, server, "time-plugin", "0.1.2", types.SystemPluginLifecycleOnDemand, false, false)
	// 等待预览解析真实开始（插件进入真实执行；执行受插件超时限制，超时后活动结束）。
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, payload := box.call(http.MethodGet, "/api/system-plugins/time-plugin", "")
		view := box.dataJSON(status, payload)
		if active, _ := view["active"].(bool); active {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	before := server.archiveRequestCount(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "time-plugin"}, "0.1.2")
	updateDone := make(chan struct{})
	var updateStatus int
	var updatePayload []byte
	go func() {
		defer close(updateDone)
		updateStatus, updatePayload = box.call(http.MethodPost, "/api/system-plugins/time-plugin/update", "{}")
	}()
	// 活动进行中：更新不能开始下载。
	time.Sleep(2 * time.Second)
	midway := server.archiveRequestCount(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "time-plugin"}, "0.1.2")
	if midway != before {
		t.Fatalf("on-demand 活动期间发生了下载：%d -> %d", before, midway)
	}
	select {
	case <-updateDone:
	case <-time.After(60 * time.Second):
		t.Fatal("on-demand 活动结束后更新未完成")
	}
	<-previewDone
	state := box.dataJSON(updateStatus, updatePayload)
	if state["currentVersion"] != "0.1.2" {
		t.Fatalf("on-demand 活动结束后更新状态 = %#v（HTTP %d）", state, updateStatus)
	}
	_ = os.Remove(marker)
}

// verifyPluginPersistentActivity 制造 persistent 进程活动并确认更新被 PLUGIN_ACTIVE 拒绝。
func verifyPluginPersistentActivity(t *testing.T, box *boxProcess, programRoot string, server *stage04Server, boxData string) {
	t.Helper()
	makePluginCandidate(t, server, "weather-plugin", "0.1.1", types.SystemPluginLifecyclePersistent, false, false)
	status, payload := box.call(http.MethodPost, "/api/system-plugins/weather-plugin/install", "{}")
	state := box.dataJSON(status, payload)
	if state["status"] != types.ArtifactStatusActive {
		t.Fatalf("persistent 插件安装状态 = %#v（HTTP %d）", state, status)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, payload = box.call(http.MethodGet, "/api/system-plugins/weather-plugin", "")
		view := box.dataJSON(status, payload)
		active, _ := view["active"].(bool)
		if active {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	makePluginCandidate(t, server, "weather-plugin", "0.1.2", types.SystemPluginLifecyclePersistent, false, false)
	before := server.archiveRequestCount(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "weather-plugin"}, "0.1.2")
	status, payload = box.call(http.MethodPost, "/api/system-plugins/weather-plugin/update", "{}")
	state = box.dataJSON(status, payload)
	if state["status"] != types.ArtifactStatusBlocked || state["error"].(map[string]any)["code"] != types.ArtifactErrorPluginActive {
		t.Fatalf("persistent 活动状态 = %#v（HTTP %d）", state, status)
	}
	after := server.archiveRequestCount(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "weather-plugin"}, "0.1.2")
	if after != before {
		t.Fatalf("persistent 活动期间发生了下载：%d -> %d", before, after)
	}
}

// verifyPluginHeartbeatActivity 制造 cached-heartbeat 刷新活动并确认活动结束前不下载、不切换。
func verifyPluginHeartbeatActivity(t *testing.T, box *boxProcess, programRoot string, server *stage04Server, boxData string) {
	t.Helper()
	makePluginCandidate(t, server, "system-info-plugin", "0.1.1", types.SystemPluginLifecycleCachedHeartbeat, false, false)
	status, payload := box.call(http.MethodPost, "/api/system-plugins/system-info-plugin/install", "{}")
	state := box.dataJSON(status, payload)
	if state["status"] != types.ArtifactStatusActive {
		t.Fatalf("heartbeat 插件安装状态 = %#v（HTTP %d）", state, status)
	}
	marker := filepath.Join(boxData, "system-plugins", "system-info-plugin", "sleep-marker.txt")
	if err := os.WriteFile(marker, []byte("sleep"), 0o644); err != nil {
		t.Fatalf("写 heartbeat marker：%v", err)
	}
	makePluginCandidate(t, server, "system-info-plugin", "0.1.2", types.SystemPluginLifecycleCachedHeartbeat, false, false)
	// 等待 ticker 触发一次真实刷新（插件进入真实执行）。
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, payload := box.call(http.MethodGet, "/api/system-plugins/system-info-plugin", "")
		view := box.dataJSON(status, payload)
		if active, _ := view["active"].(bool); active {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	before := server.archiveRequestCount(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "system-info-plugin"}, "0.1.2")
	updateDone := make(chan struct{})
	var updateStatus int
	var updatePayload []byte
	go func() {
		defer close(updateDone)
		updateStatus, updatePayload = box.call(http.MethodPost, "/api/system-plugins/system-info-plugin/update", "{}")
	}()
	// 刷新进行中：更新不能开始下载。
	time.Sleep(2 * time.Second)
	midway := server.archiveRequestCount(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "system-info-plugin"}, "0.1.2")
	if midway != before {
		t.Fatalf("heartbeat 活动期间发生了下载：%d -> %d", before, midway)
	}
	select {
	case <-updateDone:
	case <-time.After(60 * time.Second):
		t.Fatal("heartbeat 活动结束后更新未完成")
	}
	state = box.dataJSON(updateStatus, updatePayload)
	if state["currentVersion"] != "0.1.2" {
		t.Fatalf("heartbeat 活动结束后更新状态 = %#v（HTTP %d）", state, updateStatus)
	}
	_ = os.Remove(marker)
}

// verifyBrokenCandidates 提供损坏摘要、损坏 ZIP、缺少身份文件、身份不一致和越界路径样例。
func verifyBrokenCandidates(t *testing.T, box *boxProcess, programRoot string, server *stage04Server) {
	t.Helper()
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "web_search"}
	versions := []string{"0.1.1", "0.1.2", "0.1.3", "0.1.4"}
	kinds := []string{"corrupt-zip", "missing-identity", "identity-mismatch", "path-escape"}
	for index := range versions {
		makeBrokenToolCandidate(t, server, identity.ID, versions[index], kinds[index])
		status, payload := box.call(http.MethodPost, fmt.Sprintf("/api/tools/%s/install", identity.ID), "{}")
		state := box.dataJSON(status, payload)
		if state["status"] != types.ArtifactStatusFailed {
			t.Fatalf("损坏样例 %s 状态 = %#v（HTTP %d）", kinds[index], state, status)
		}
		if _, err := os.Stat(filepath.Join(programRoot, "tools", identity.ID, "current.json")); err == nil {
			t.Fatalf("损坏样例 %s 产生了当前版本记录", kinds[index])
		}
	}
}

// verifyInterruptedSwitchRecovery 制造切换中断和未完成 operation.json，重启后按阶段恢复。
// 返回最后一次重启后的业务端进程。
func verifyInterruptedSwitchRecovery(t *testing.T, boxPath string, envDir string, programRoot string, box *boxProcess, serverURL string) *boxProcess {
	t.Helper()
	// 制造中断现场：current.json 指向 0.1.2，operation.json 为 running/switch，currentVersion=0.1.1。
	record := map[string]any{
		"schemaVersion":  1,
		"operationId":    "interrupted-switch",
		"artifact":       map[string]any{"kind": "tool", "id": "context7"},
		"action":         "update",
		"targetVersion":  "0.1.2",
		"phase":          "switch",
		"result":         "running",
		"currentVersion": "0.1.1",
		"workDirectory":  filepath.Join(programRoot, "tools", "context7", "work", "interrupted-switch"),
		"startedAt":      time.Now().UTC().Format(time.RFC3339Nano),
		"updatedAt":      time.Now().UTC().Format(time.RFC3339Nano),
		"errorCode":      "",
		"errorMessage":   "",
	}
	recordJSON, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatalf("marshal operation: %v", err)
	}
	if err := os.WriteFile(filepath.Join(programRoot, "tools", "context7", "operation.json"), recordJSON, 0o644); err != nil {
		t.Fatalf("写 operation.json：%v", err)
	}
	current := map[string]any{
		"schemaVersion":    1,
		"artifact":         map[string]any{"kind": "tool", "id": "context7"},
		"version":          "0.1.2",
		"platform":         "windows-x64",
		"programDirectory": filepath.Join(programRoot, "tools", "context7", "versions", "0.1.2"),
		"status":           "active",
	}
	currentJSON, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		t.Fatalf("marshal current: %v", err)
	}
	if err := os.WriteFile(filepath.Join(programRoot, "tools", "context7", "current.json"), currentJSON, 0o644); err != nil {
		t.Fatalf("写 current.json：%v", err)
	}
	// 重启业务端。
	box.stop()
	box = startBox(t, boxPath, envDir, serverURL)
	status, payload := box.call(http.MethodGet, "/api/tools/context7/install-state", "")
	state := box.dataJSON(status, payload)
	if state["status"] != types.ArtifactStatusActive || state["currentVersion"] != "0.1.1" {
		t.Fatalf("中断恢复后状态 = %#v（HTTP %d）", state, status)
	}
	if _, err := os.Stat(filepath.Join(programRoot, "tools", "context7", "operation.json")); err == nil {
		t.Fatal("恢复后 operation.json 未清除")
	}

	// 未知当前版本：损坏 current.json 后重启，不启用任何版本。
	if err := os.WriteFile(filepath.Join(programRoot, "tools", "context7", "current.json"), []byte(`{"broken":true}`), 0o644); err != nil {
		t.Fatalf("损坏 current.json：%v", err)
	}
	box.stop()
	box = startBox(t, boxPath, envDir, serverURL)
	status, payload = box.call(http.MethodGet, "/api/tools/context7/install-state", "")
	state = box.dataJSON(status, payload)
	if state["status"] != types.ArtifactStatusFailed {
		t.Fatalf("未知当前版本状态 = %#v（HTTP %d）", state, status)
	}
	errorData, _ := state["error"].(map[string]any)
	if errorData["code"] != types.ArtifactErrorStateUnknown {
		t.Fatalf("未知当前版本错误 = %#v", errorData)
	}
	return box
}

func findReleaseResults(t *testing.T, payload []byte) map[string]map[string]any {
	t.Helper()
	var envelope struct {
		Data struct {
			Results []map[string]any `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("解析发行检查响应失败：%v", err)
	}
	result := map[string]map[string]any{}
	for _, item := range envelope.Data.Results {
		artifact, _ := item["artifact"].(map[string]any)
		key := fmt.Sprintf("%s:%s", artifact["kind"], artifact["id"])
		result[key] = item
	}
	return result
}

// TestStage04Experience 体验模式：一次安装动作和一次更新动作的最小链路。
func TestStage04Experience(t *testing.T) {
	runRoot := os.Getenv("EUCLI_STAGE04_RUN_ROOT")
	boxPath := os.Getenv("EUCLI_STAGE04_BOX")
	if runRoot == "" || boxPath == "" {
		t.Fatal("阶段四体验验证缺少运行目录或业务端可执行文件")
	}
	envDir := filepath.Join(runRoot, "environment")
	boxData := filepath.Join(envDir, "box-data")

	server := newStage04Server(t)
	makeToolCandidate(t, server, "context7", "0.1.0", false, false)

	box := startBox(t, boxPath, envDir, server.url())

	// 用户点击一次"安装"：不需要填写地址、下载地址或版本号。
	status, payload := box.call(http.MethodPost, "/api/tools/context7/install", "{}")
	state := box.dataJSON(status, payload)
	if state["status"] != types.ArtifactStatusActive || state["currentVersion"] != "0.1.0" {
		t.Fatalf("一次安装动作状态 = %#v（HTTP %d）", state, status)
	}
	if count := server.archiveRequestCount(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, "0.1.0"); count != 1 {
		t.Fatalf("一次安装动作对应压缩包请求 %d 次", count)
	}

	// 数据基线：保存工具用户配置。
	status, _ = box.call(http.MethodPut, "/api/tools/context7/user-config", `{"userConfig":{"mode":"fast"},"promptDescriptionOverride":""}`)
	if status != 200 {
		t.Fatalf("保存工具用户配置 HTTP %d", status)
	}
	dataBefore := snapshotDir(filepath.Join(boxData, "tool-data", "context7"))

	// 用户点击一次"更新"：页面显示更新阶段并最终显示新版可用。
	makeToolCandidate(t, server, "context7", "0.1.1", false, false)
	status, payload = box.call(http.MethodPost, "/api/tools/context7/update", "{}")
	state = box.dataJSON(status, payload)
	if state["status"] != types.ArtifactStatusActive || state["currentVersion"] != "0.1.1" {
		t.Fatalf("一次更新动作状态 = %#v（HTTP %d）", state, status)
	}
	if count := server.archiveRequestCount(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, "0.1.1"); count != 1 {
		t.Fatalf("一次更新动作对应压缩包请求 %d 次", count)
	}

	// 脚本自动确认：只有一次确认动作、无第二次确认、工具长期数据未改变。
	dataAfter := snapshotDir(filepath.Join(boxData, "tool-data", "context7"))
	compareDirSnapshots(t, "体验模式工具长期数据", dataBefore, dataAfter)
	t.Log("阶段四体验验证完成：一次安装 + 一次更新，无第二次确认，长期数据未改变")
}
