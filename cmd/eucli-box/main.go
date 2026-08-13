package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"eucli-box/internal/boxrelease"
	"eucli-box/pkg/localrun"
	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
	accesssystem "eucli-box/src/access-system"
	agentruntime "eucli-box/src/agent-runtime-system"
	aiassist "eucli-box/src/ai-assist-system"
	datamigration "eucli-box/src/data-migration-system"
	datastorage "eucli-box/src/data-storage-system"
	gateway "eucli-box/src/gateway-system"
	modelprovider "eucli-box/src/model-provider-system"
	networkrequest "eucli-box/src/network-request-system"
	permission "eucli-box/src/permission-system"
	placeholdersystem "eucli-box/src/placeholder-system"
	releasechecksystem "eucli-box/src/release-check-system"
	roleprompt "eucli-box/src/role-prompt-system"
	systemplugin "eucli-box/src/system-plugin-system"
	toolcalling "eucli-box/src/tool-calling-system"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	localConfig, err := loadLocalRunConfig()
	if err != nil {
		return err
	}
	if localConfig.Enabled && (runtime.GOOS != "windows" || runtime.GOARCH != "amd64") {
		return localrun.ErrWindowsOnly
	}
	boxRelease, err := boxrelease.Load()
	if err != nil {
		return fmt.Errorf("load eucli-box release metadata: %w", err)
	}
	log.Printf("eucli-box v%s", boxRelease.Version)

	networkSystem, err := networkrequest.NewSystem(networkrequest.Config{MaxTimeout: time.Duration(types.ModelRequestCompletionTimeoutMaxMs) * time.Millisecond})
	if err != nil {
		return fmt.Errorf("start network request system: %w", err)
	}
	log.Printf("[1/13] network-request-system ✓")

	programRoot := strings.TrimSpace(os.Getenv("EUCLI_BOX_PROGRAM_ROOT"))
	if programRoot != "" {
		programRoot, err = filepath.Abs(programRoot)
		if err != nil {
			return fmt.Errorf("program root is invalid: %w", err)
		}
		programRoot = filepath.Clean(programRoot)
	}
	officialDoer := boxOfficialHTTPDoer{network: networkSystem}
	apiBaseURL := strings.TrimSpace(os.Getenv("EUCLI_BOX_RELEASE_API_BASE"))
	if apiBaseURL == "" {
		apiBaseURL = "https://api.github.com"
	}
	officialChecker, err := releasecheck.New(releasecheck.Config{Client: officialDoer, APIBaseURL: apiBaseURL})
	if err != nil {
		return fmt.Errorf("create official candidate checker: %w", err)
	}
	log.Printf("[2/13] official candidate reader %s", programStatusLabel(programRoot))

	dataDir := envOrDefault("EUCLI_BOX_DATA_DIR", "data")
	if localConfig.Enabled {
		dataDir = localConfig.DataDir
	}
	dataLock, err := localrun.AcquireDataLock(dataDir)
	if err != nil {
		return err
	}
	defer dataLock.Release()

	var dataIdentity localrun.DataIdentityRecord
	var processStartedAt time.Time
	if localConfig.Enabled {
		dataIdentity, err = localrun.EnsureDataIdentity(dataDir)
		if err != nil {
			return err
		}
		if dataIdentity.DataIdentity != localConfig.DataIdentity {
			return fmt.Errorf("LOCAL_BOX_DATA_IDENTITY_MISMATCH")
		}
		processStartedAt, err = localrun.ProcessStartedAt(os.Getpid())
		if err != nil {
			return err
		}
	}
	migrationSession, err := datamigration.Prepare(ctx, dataDir, boxRelease.DataVersion)
	if err != nil {
		return fmt.Errorf("数据迁移准备失败：%w", err)
	}
	if err := migrationSession.Run(ctx); err != nil {
		return fmt.Errorf("数据迁移执行失败：%w", err)
	}
	outcome := migrationSession.Outcome()
	log.Printf("数据迁移结果：%s（数据版本 %s → %s）", outcome.State, outcome.From, outcome.To)
	migrationCompleted := false
	defer func() {
		if !migrationCompleted {
			if recoverErr := migrationSession.Recover(context.Background()); recoverErr != nil {
				log.Printf("数据恢复失败：%v（现场保留在迁移工作区，需要人工处理）", recoverErr)
			}
		}
	}()
	toolBodiesRoot := ""
	toolProgramRoot := ""
	if programRoot != "" {
		toolBodiesRoot = filepath.Join(programRoot, "tools")
		toolProgramRoot = toolBodiesRoot
	}
	storageSystem, err := datastorage.NewSystem(datastorage.Config{RootDir: dataDir, ToolBodiesRoot: toolBodiesRoot})
	if err != nil {
		return fmt.Errorf("start data storage system: %w", err)
	}
	if err := storageSystem.Initialize(ctx); err != nil {
		return fmt.Errorf("initialize data storage system: %w", err)
	}
	log.Printf("[3/13] data-storage-system     ✓  (%s)", dataDir)

	providerSystem, err := modelprovider.NewSystem(modelprovider.Config{}, networkSystem, storageSystem)
	if err != nil {
		return fmt.Errorf("start model provider system: %w", err)
	}
	log.Printf("[4/13] model-provider-system   ✓")

	roleSystem, err := roleprompt.NewSystem(roleprompt.Config{}, storageSystem, providerSystem)
	if err != nil {
		return fmt.Errorf("start role prompt system: %w", err)
	}
	log.Printf("[5/13] role-prompt-system      ✓")

	permissionSystem, err := permission.NewSystem(permission.Config{}, roleSystem)
	if err != nil {
		return fmt.Errorf("start permission system: %w", err)
	}
	log.Printf("[6/13] permission-system       ✓")

	toolSystem, err := toolcalling.NewSystem(toolcalling.Config{BoxVersion: boxRelease.Version, ProgramRoot: toolProgramRoot, Candidates: officialChecker, HTTPClient: officialDoer}, permissionSystem, storageSystem)
	if err != nil {
		return fmt.Errorf("start tool calling system: %w", err)
	}
	log.Printf("[7/13] tool-calling-system     ✓")

	pluginSourceDir := ""
	pluginDataDir := filepath.Join(dataDir, "system-plugins")
	if programRoot != "" {
		pluginSourceDir = filepath.Join(programRoot, "system-plugins")
	}
	systemPluginSystem, err := systemplugin.NewSystem(systemplugin.Config{SourceDir: pluginSourceDir, DataDir: pluginDataDir, BoxVersion: boxRelease.Version, ProgramRoot: programRoot, Candidates: officialChecker, HTTPClient: officialDoer})
	if err != nil {
		return fmt.Errorf("start system plugin system: %w", err)
	}
	if err := systemPluginSystem.Start(ctx); err != nil {
		return fmt.Errorf("initialize system plugin system: %w", err)
	}
	log.Printf("[8/13] system-plugin-system    ✓")

	placeholderSystem, err := placeholdersystem.NewSystem(placeholdersystem.Config{RootDir: dataDir, SystemPlugins: systemPluginSystem})
	if err != nil {
		return fmt.Errorf("start placeholder system: %w", err)
	}
	log.Printf("[9/13] placeholder-system      ✓")

	runtimeSystem, err := agentruntime.NewSystem(agentruntime.Config{}, storageSystem, roleSystem, providerSystem, toolSystem, placeholderSystem)
	if err != nil {
		return fmt.Errorf("start agent runtime system: %w", err)
	}
	log.Printf("[10/13] agent-runtime-system    ✓")

	assistSystem, err := aiassist.NewSystem(aiassist.Config{}, storageSystem, providerSystem)
	if err != nil {
		return fmt.Errorf("start ai assist system: %w", err)
	}
	log.Printf("[11/13] ai-assist-system        ✓")

	releaseCheckSystem, err := releasechecksystem.NewSystemWithChecker(releasechecksystem.Config{BoxVersion: boxRelease.Version}, officialChecker, toolSystem, systemPluginSystem, boxRelease.Version)
	if err != nil {
		return fmt.Errorf("start release check system: %w", err)
	}
	log.Printf("[12/13] release-check-system    ✓")

	accessSystem, err := accesssystem.NewSystem(dataDir)
	if err != nil {
		return fmt.Errorf("start access system: %w", err)
	}
	if err := accesssystem.MigrateLegacyConfig(ctx, accessSystem, dataDir); err != nil {
		return fmt.Errorf("migrate legacy access config: %w", err)
	}
	log.Printf("[12.5/13] access-system            ✓")

	busyKey := ""
	if !localConfig.Enabled && readBoxKey(dataDir) != "" {
		busyKey = " (key: active)"
	}
	gatewayConfig := gateway.Config{Addr: envOrDefault("EUCLI_BOX_ADDR", "127.0.0.1:8765"), Key: readBoxKey(dataDir), BoxVersion: boxRelease.Version, Access: accessSystem}
	if localConfig.Enabled {
		gatewayConfig = gateway.Config{
			Addr:              localConfig.Address,
			BoxVersion:        boxRelease.Version,
			LocalRun:          true,
			LocalInstallID:    localConfig.InstallIdentity,
			LocalDataID:       localConfig.DataIdentity,
			LocalRunID:        localConfig.RunIdentity,
			LocalCredential:   localConfig.SessionCredential,
			LocalProcessID:    os.Getpid(),
			LocalProcessStart: processStartedAt,
			LocalStop:         stop,
			Access:            accessSystem,
		}
	}
	gatewaySystem, err := gateway.NewSystem(gatewayConfig, runtimeSystem, roleSystem, storageSystem, storageSystem, providerSystem, toolSystem, storageSystem, storageSystem, storageSystem, placeholderSystem, systemPluginSystem, assistSystem, releaseCheckSystem)
	if err != nil {
		return fmt.Errorf("start gateway system: %w", err)
	}
	log.Printf("[13/13] gateway-system         ✓%s", busyKey)

	log.Printf("eucli-box v%s is starting on %s ...", boxRelease.Version, gatewayConfig.Addr)
	endpoint := "http://" + gatewayConfig.Addr
	accessSystem.SetHandler(gatewaySystem.LongTermHandler())
	if localConfig.Enabled {
		started, startErr := gatewaySystem.StartLocal(ctx)
		if startErr != nil {
			return fmt.Errorf("start local gateway listener: %w", startErr)
		}
		endpoint = started.Endpoint
		accessSystem.SetLocalEntrypointPort(entrypointPort(endpoint))
		registration := localrun.Registration{
			SchemaVersion:     localrun.RegistrationSchemaVersion,
			InstallIdentity:   localConfig.InstallIdentity,
			DataIdentity:      dataIdentity.DataIdentity,
			RunIdentity:       localConfig.RunIdentity,
			Endpoint:          endpoint,
			SessionCredential: localConfig.SessionCredential,
			ProcessID:         os.Getpid(),
			ProcessStartedAt:  processStartedAt,
			BoxVersion:        boxRelease.Version,
			Status:            localrun.RegistrationStatusRunning,
		}
		if err := localrun.WriteRegistration(localConfig.RegistrationPath, registration); err != nil {
			_ = gatewaySystem.Shutdown(context.Background())
			return fmt.Errorf("write local runtime registration: %w", err)
		}
		defer func() {
			if err := localrun.DeleteRegistration(localConfig.RegistrationPath); err != nil {
				_ = localrun.MarkRegistrationStale(localConfig.RegistrationPath)
			}
		}()
		if err := writeLocalReady(boxRelease.Version, endpoint, localConfig); err != nil {
			_ = gatewaySystem.Shutdown(context.Background())
			return err
		}
		if err := migrationSession.Complete(ctx); err != nil {
			return fmt.Errorf("数据迁移收尾失败：%w", err)
		}
		migrationCompleted = true
	} else {
		if err := gatewaySystem.Start(ctx); err != nil {
			return fmt.Errorf("start gateway listener: %w", err)
		}
		if err := migrationSession.Complete(ctx); err != nil {
			return fmt.Errorf("数据迁移收尾失败：%w", err)
		}
		migrationCompleted = true
	}
	accessSystem.Start(ctx)
	log.Printf("eucli-box v%s is ready — listening on %s", boxRelease.Version, endpoint)

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	accessSystem.Shutdown(shutdownCtx)
	if err := gatewaySystem.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown gateway system: %w", err)
	}
	if err := systemPluginSystem.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown system plugin system: %w", err)
	}
	return nil
}

type localRunConfig struct {
	Enabled           bool
	InstallIdentity   string
	DataIdentity      string
	RunIdentity       string
	SessionCredential string
	DataDir           string
	RegistrationPath  string
	Address           string
	TempDir           string
}

func loadLocalRunConfig() (localRunConfig, error) {
	if os.Getenv("EUCLI_BOX_LOCAL_RUN") != "1" {
		return localRunConfig{}, nil
	}
	value := localRunConfig{
		Enabled:           true,
		InstallIdentity:   strings.TrimSpace(os.Getenv("EUCLI_BOX_INSTALL_ID")),
		DataIdentity:      strings.TrimSpace(os.Getenv("EUCLI_BOX_DATA_ID")),
		RunIdentity:       strings.TrimSpace(os.Getenv("EUCLI_BOX_RUN_ID")),
		SessionCredential: strings.TrimSpace(os.Getenv("EUCLI_BOX_SESSION_CREDENTIAL")),
		DataDir:           strings.TrimSpace(os.Getenv("EUCLI_BOX_DATA_DIR")),
		RegistrationPath:  strings.TrimSpace(os.Getenv("EUCLI_BOX_REGISTRATION_PATH")),
		Address:           strings.TrimSpace(os.Getenv("EUCLI_BOX_ADDR")),
		TempDir:           strings.TrimSpace(os.Getenv("TEMP")),
	}
	for _, item := range []struct {
		value string
		name  string
	}{{value.InstallIdentity, "EUCLI_BOX_INSTALL_ID"}, {value.DataIdentity, "EUCLI_BOX_DATA_ID"}, {value.RunIdentity, "EUCLI_BOX_RUN_ID"}, {value.SessionCredential, "EUCLI_BOX_SESSION_CREDENTIAL"}, {value.DataDir, "EUCLI_BOX_DATA_DIR"}, {value.RegistrationPath, "EUCLI_BOX_REGISTRATION_PATH"}, {value.Address, "EUCLI_BOX_ADDR"}, {value.TempDir, "TEMP"}} {
		if item.value == "" {
			return localRunConfig{}, fmt.Errorf("受托启动资料缺少 %s", item.name)
		}
	}
	if strings.TrimSpace(os.Getenv("TMP")) == "" {
		return localRunConfig{}, fmt.Errorf("受托启动资料缺少 TMP")
	}
	if value.Address != "127.0.0.1:0" {
		return localRunConfig{}, fmt.Errorf("受托模式地址必须为 127.0.0.1:0")
	}
	if !filepath.IsAbs(value.DataDir) || !filepath.IsAbs(value.RegistrationPath) {
		return localRunConfig{}, fmt.Errorf("受托启动目录必须使用绝对路径")
	}
	if err := localrun.ValidateIdentity(value.InstallIdentity, localrun.IdentityKindInstall); err != nil {
		return localRunConfig{}, err
	}
	if err := localrun.ValidateIdentity(value.DataIdentity, localrun.IdentityKindData); err != nil {
		return localRunConfig{}, err
	}
	if err := localrun.ValidateIdentity(value.RunIdentity, localrun.IdentityKindRun); err != nil {
		return localRunConfig{}, err
	}
	if err := localrun.ValidateIdentity(value.SessionCredential, localrun.IdentityKindSession); err != nil {
		return localRunConfig{}, err
	}
	return value, nil
}

func writeLocalReady(version string, endpoint string, config localRunConfig) error {
	payload, err := json.Marshal(map[string]string{
		"type":            "local-box-ready",
		"endpoint":        endpoint,
		"installIdentity": config.InstallIdentity,
		"dataIdentity":    config.DataIdentity,
		"runIdentity":     config.RunIdentity,
		"version":         version,
	})
	if err != nil {
		return fmt.Errorf("生成受托 ready 资料失败：%w", err)
	}
	if _, err := fmt.Fprintln(os.Stdout, string(payload)); err != nil {
		return fmt.Errorf("输出受托 ready 资料失败：%w", err)
	}
	return nil
}

func envOrDefault(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

// entrypointPort 从受托本机入口地址解析真实端口，用于长期端口冲突检查。
func entrypointPort(endpoint string) int {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		return 0
	}
	return port
}

func readBoxKey(dataDir string) string {
	keyFile := filepath.Join(dataDir, "meta", "box.key")
	payload, err := os.ReadFile(keyFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(payload))
}

func programStatusLabel(programRoot string) string {
	if programRoot == "" {
		return "(development path)"
	}
	return "✓  (" + programRoot + ")"
}

// boxOfficialHTTPDoer 是业务端网络系统对官方发行检查的只读适配器；
// 只允许固定官方 HTTPS 地址和隔离验证地址，不能成为通用请求通道。
type boxOfficialHTTPDoer struct {
	network interface {
		Do(ctx context.Context, request types.HTTPRequest) (types.HTTPResponse, error)
	}
}

func (d boxOfficialHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	target := request.URL.String()
	allowed := strings.HasPrefix(target, "https://api.github.com/") || strings.HasPrefix(target, "https://github.com/") || strings.HasPrefix(target, "https://raw.githubusercontent.com/") || strings.HasPrefix(target, "http://127.0.0.1:")
	if !allowed {
		return nil, fmt.Errorf("发行检查只能访问固定官方地址")
	}
	headers := make(map[string]string, len(request.Header))
	for name, values := range request.Header {
		if len(values) > 0 {
			headers[name] = values[0]
		}
	}
	response, err := d.network.Do(request.Context(), types.HTTPRequest{Method: request.Method, URL: target, Headers: headers})
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: response.StatusCode,
		Header:     http.Header(response.Headers),
		Body:       io.NopCloser(bytes.NewReader(response.Body)),
		Request:    request,
	}, nil
}

var _ release.HTTPDoer = boxOfficialHTTPDoer{}
