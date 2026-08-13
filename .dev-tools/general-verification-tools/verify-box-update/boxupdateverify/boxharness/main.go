// boxharness 是可编排的业务端替身，用于阶段七业务端更新链验证。
// 行为由程序同目录的 scenario.json 编排；读 EUCLI_BOX_* 环境变量，
// 模拟首装数据版本、迁移现场、运行登记、ready 行与受托网关端点。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"eucli-box/pkg/localrun"
	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

// scenario 是替身的编排事实。
type scenario struct {
	Version              string                       `json:"version"`
	InitialDataVersion   string                       `json:"initialDataVersion"`
	DataVersion          string                       `json:"dataVersion"`
	ActiveWork           int                          `json:"activeWork"`
	ShutdownStatus       int                          `json:"shutdownStatus"`
	ExitAfterReady       bool                         `json:"exitAfterReady"`
	MigrationBehavior    string                       `json:"migrationBehavior"`
	RecoverOnSecondStart bool                         `json:"recoverOnSecondStart"`
	ClientCompatibility  *types.EucliBoxCompatibility `json:"clientCompatibility,omitempty"`
}

type harness struct {
	scenario         scenario
	installID        string
	dataID           string
	runID            string
	sessionCred      string
	dataDir          string
	registrationPath string
	addr             string
}

var (
	processID    int
	processStart time.Time
	shutdownCh   = make(chan struct{})
	logPath      string
)

func logf(format string, args ...any) {
	if logPath == "" {
		logPath = filepath.Join(filepath.Dir(os.Args[0]), "harness.log")
	}
	payload := fmt.Sprintf("%s boxharness: "+format+"\n", append([]any{time.Now().UTC().Format("15:04:05.000")}, args...)...)
	_ = os.WriteFile(logPath, []byte(payload), 0o644)
}

func main() {
	processID = os.Getpid()
	startedAt, err := localrun.ProcessStartedAt(processID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "boxharness:", err)
		os.Exit(1)
	}
	processStart = startedAt
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "boxharness:", err)
		os.Exit(1)
	}
}

func run() error {
	exeDir := filepath.Dir(os.Args[0])
	scenarioPath := filepath.Join(exeDir, "scenario.json")
	var cfg scenario
	payload, err := os.ReadFile(scenarioPath)
	if err == nil {
		if err := json.Unmarshal(payload, &cfg); err != nil {
			return fmt.Errorf("scenario.json 无效：%w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("读取 scenario.json 失败：%w", err)
	}
	if cfg.Version == "" {
		return fmt.Errorf("scenario 缺少版本声明")
	}
	if cfg.ShutdownStatus == 0 {
		cfg.ShutdownStatus = http.StatusOK
	}
	h := &harness{
		scenario:         cfg,
		installID:        os.Getenv("EUCLI_BOX_INSTALL_ID"),
		dataID:           os.Getenv("EUCLI_BOX_DATA_ID"),
		runID:            os.Getenv("EUCLI_BOX_RUN_ID"),
		sessionCred:      os.Getenv("EUCLI_BOX_SESSION_CREDENTIAL"),
		dataDir:          os.Getenv("EUCLI_BOX_DATA_DIR"),
		registrationPath: os.Getenv("EUCLI_BOX_REGISTRATION_PATH"),
		addr:             os.Getenv("EUCLI_BOX_ADDR"),
	}
	if os.Getenv("EUCLI_BOX_LOCAL_RUN") != "1" {
		return fmt.Errorf("EUCLI_BOX_LOCAL_RUN 必须为 1")
	}
	for label, value := range map[string]string{
		"EUCLI_BOX_INSTALL_ID": h.installID, "EUCLI_BOX_DATA_ID": h.dataID, "EUCLI_BOX_RUN_ID": h.runID,
		"EUCLI_BOX_SESSION_CREDENTIAL": h.sessionCred, "EUCLI_BOX_DATA_DIR": h.dataDir,
		"EUCLI_BOX_REGISTRATION_PATH": h.registrationPath, "EUCLI_BOX_ADDR": h.addr,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("缺少环境变量 %s", label)
		}
	}

	if err := h.applyMigrationBehavior(); err != nil {
		return err
	}
	logf("started version=%s", h.scenario.Version)
	if err := h.ensureInitialDataVersion(); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("监听本机端口失败：%w", err)
	}
	defer listener.Close()
	endpoint := "http://127.0.0.1:" + strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)

	registration := localrun.Registration{
		SchemaVersion:     localrun.RegistrationSchemaVersion,
		InstallIdentity:   h.installID,
		DataIdentity:      h.dataID,
		RunIdentity:       h.runID,
		Endpoint:          endpoint,
		SessionCredential: h.sessionCred,
		ProcessID:         processID,
		ProcessStartedAt:  processStart,
		BoxVersion:        h.scenario.Version,
		Status:            localrun.RegistrationStatusRunning,
	}
	if err := localrun.WriteRegistration(h.registrationPath, registration); err != nil {
		return fmt.Errorf("写运行登记失败：%w", err)
	}

	ready := map[string]string{
		"type": "local-box-ready", "endpoint": endpoint,
		"installIdentity": h.installID, "dataIdentity": h.dataID,
		"runIdentity": h.runID, "version": h.scenario.Version,
	}
	readyPayload, _ := json.Marshal(ready)
	logf("writing registration and ready endpoint=%s", endpoint)
	fmt.Println(string(readyPayload))

	if h.scenario.ExitAfterReady {
		return nil
	}

	server := &http.Server{Handler: h.handler(endpoint)}
	go func() {
		_ = server.Serve(listener)
	}()
	select {
	case <-shutdownCh:
		logf("shutdown signal received, closing server")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		logf("server closed, exiting")
		return nil
	}
}

// applyMigrationBehavior 模拟迁移现场与恢复；返回错误时进程以非零退出。
func (h *harness) applyMigrationBehavior() error {
	bootCount := h.bootCount()
	logf("migration behavior=%s boot=%d", h.scenario.MigrationBehavior, bootCount)
	switch h.scenario.MigrationBehavior {
	case "crash-with-process":
		if bootCount <= 1 {
			if err := h.writeProcessRecord(); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "boxharness: crash after writing process record")
			os.Exit(17)
		}
		if h.scenario.RecoverOnSecondStart {
			if err := h.recoverMigration(); err != nil {
				return err
			}
		}
	case "recovery-failed":
		if bootCount <= 1 {
			if err := h.writeProcessRecord(); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "boxharness: crash after writing process record")
			os.Exit(17)
		}
		if err := h.writeStatusRecord("recovery-failed", "1.0.0"); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "boxharness: recovery failed, scene retained")
		os.Exit(18)
	case "migrate-then-exit":
		workspaceDir := datamigrationWorkspaceDir(h.dataDir)
		if err := h.writeDataVersion("1.2.0"); err != nil {
			return err
		}
		if err := writeJSONFile(filepath.Join(workspaceDir, "status.json"), map[string]any{
			"schemaVersion": 1, "outcome": "migrated", "fromVersion": "1.0.0", "targetVersion": "1.2.0",
			"currentDataVersion": "1.2.0", "stepIDs": []string{"1.0.0-to-1.1.0"}, "completed": true,
			"detail": "每级迁移均完成核对", "updatedAt": time.Now().UTC().Format(time.RFC3339Nano),
		}); err != nil {
			return err
		}
		h.scenario.ExitAfterReady = true
	}
	return nil
}

func (h *harness) bootCount() int {
	// 计数文件放在替身程序所在版本目录内，每个版本独立计数；
	// 安装与更新流程中旧版替身与新版替身各自记录自己的启动次数。
	path := filepath.Join(filepath.Dir(os.Args[0]), "harness-boot.json")
	payload, err := os.ReadFile(path)
	count := 0
	if err == nil {
		var record struct {
			Count int `json:"count"`
		}
		if json.Unmarshal(payload, &record) == nil {
			count = record.Count
		}
	}
	count++
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(fmt.Sprintf("{\n  \"count\": %d\n}\n", count)), 0o644)
	return count
}

func (h *harness) writeProcessRecord() error {
	workspaceDir := datamigrationWorkspaceDir(h.dataDir)
	record := map[string]any{
		"schemaVersion": 1, "fromVersion": "1.0.0", "targetVersion": "1.2.0",
		"stepIDs": []string{"1.0.0-to-1.1.0"}, "currentIndex": 0, "stepResults": []any{},
		"backup":    map[string]any{"runID": "harness", "manifest": "backup/run-harness/manifest.json", "verified": true},
		"directive": "continue", "startedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"updatedAt": time.Now().UTC().Format(time.RFC3339Nano),
	}
	return writeJSONFile(filepath.Join(workspaceDir, "process.json"), record)
}

func (h *harness) recoverMigration() error {
	workspaceDir := datamigrationWorkspaceDir(h.dataDir)
	if err := os.Remove(filepath.Join(workspaceDir, "process.json")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除未完成迁移记录失败：%w", err)
	}
	return h.writeStatusRecord("recovered", "1.0.0")
}

func (h *harness) writeStatusRecord(outcome string, currentDataVersion string) error {
	workspaceDir := datamigrationWorkspaceDir(h.dataDir)
	return writeJSONFile(filepath.Join(workspaceDir, "status.json"), map[string]any{
		"schemaVersion": 1, "outcome": outcome, "fromVersion": "1.0.0", "targetVersion": "1.2.0",
		"currentDataVersion": currentDataVersion, "stepIDs": []string{}, "completed": false,
		"detail": "boxharness 模拟", "updatedAt": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// ensureInitialDataVersion 模拟业务端首装数据版本事实。
func (h *harness) ensureInitialDataVersion() error {
	versionPath := filepath.Join(h.dataDir, "meta", "version.json")
	if _, err := os.Stat(versionPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	version := h.scenario.InitialDataVersion
	if version == "" {
		version = h.scenario.DataVersion
	}
	if version == "" {
		return fmt.Errorf("scenario 缺少数据版本声明")
	}
	return h.writeDataVersion(version)
}

func (h *harness) writeDataVersion(version string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return writeJSONFile(filepath.Join(h.dataDir, "meta", "version.json"), map[string]any{
		"version": version, "createdAt": now, "updatedAt": now,
	})
}

func (h *harness) handler(endpoint string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/local-run", func(w http.ResponseWriter, r *http.Request) {
		writeData(w, map[string]any{
			"installIdentity": h.installID, "dataIdentity": h.dataID, "runIdentity": h.runID,
			"processId": processID, "processStartedAt": processStart.Format(time.RFC3339Nano),
			"version": h.scenario.Version,
		})
	})
	mux.HandleFunc("POST /api/local-run/stop", func(w http.ResponseWriter, r *http.Request) {
		writeData(w, map[string]string{"status": "stopping"})
		go func() { shutdownCh <- struct{}{} }()
	})
	mux.HandleFunc("GET /api/box/active-work", func(w http.ResponseWriter, r *http.Request) {
		active := make([]map[string]any, 0, h.scenario.ActiveWork)
		for index := 0; index < h.scenario.ActiveWork; index++ {
			active = append(active, map[string]any{"id": fmt.Sprintf("run-%d", index), "status": "running"})
		}
		writeData(w, map[string]any{"activeWork": active})
	})
	mux.HandleFunc("POST /api/box/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if h.scenario.ShutdownStatus == http.StatusConflict {
			logf("shutdown rejected with 409")
			writeDataStatus(w, http.StatusConflict, map[string]any{"requiresConfirmation": true, "activeWork": []any{}})
			return
		}
		logf("shutdown accepted")
		writeData(w, map[string]string{"status": "shutdown_requested"})
		go func() { shutdownCh <- struct{}{} }()
	})
	mux.HandleFunc("GET /api/release", func(w http.ResponseWriter, r *http.Request) {
		info := map[string]any{
			"version": h.scenario.Version, "dataVersion": h.scenario.DataVersion,
		}
		if h.scenario.ClientCompatibility != nil {
			status := assessClientCompatibility(r, h.scenario.Version, *h.scenario.ClientCompatibility)
			info["clientCompatibility"] = status
		}
		writeData(w, info)
	})
	return mux
}

func assessClientCompatibility(r *http.Request, boxVersion string, compatibility types.EucliBoxCompatibility) map[string]any {
	version := strings.TrimSpace(r.Header.Get("X-Eucli-Studio-Version"))
	if version == "" {
		return nil
	}
	status := release.AssessEucliBoxCompatibility(version, boxVersion, compatibility)
	return map[string]any{
		"compatible": status.Compatible, "reason": status.Reason,
		"currentEucliBoxVersion":        boxVersion,
		"requiredEucliBoxCompatibility": compatibility,
	}
}

func writeData(w http.ResponseWriter, value any) {
	writeDataStatus(w, http.StatusOK, value)
}

func writeDataStatus(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": value})
}

func writeJSONFile(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func datamigrationWorkspaceDir(dataDir string) string {
	absolute, err := filepath.Abs(dataDir)
	if err != nil {
		return dataDir + ".migration"
	}
	clean := filepath.Clean(absolute)
	return filepath.Join(filepath.Dir(clean), filepath.Base(clean)+".migration")
}
