package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"eucli-box/internal/boxrelease"
	"eucli-box/pkg/datapaths"
	"eucli-box/pkg/installsource"
	"eucli-box/pkg/localrun"
	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/serviceprofile"
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
	requestrecord "eucli-box/src/request-record-system"
	releasesourcesystem "eucli-box/src/release-source-system"
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
	boxRelease, err := boxrelease.Load()
	if err != nil {
		return fmt.Errorf("load eucli-box release metadata: %w", err)
	}
	log.Printf("eucli-box v%s", boxRelease.Version)

	networkSystem, err := networkrequest.NewSystem(networkrequest.Config{MaxTimeout: time.Millisecond * time.Duration(types.ModelRequestCompletionTimeoutMaxMs)})
	if err != nil {
		return fmt.Errorf("start network request system: %w", err)
	}
	log.Printf("[1/13] network-request-system ✓")

	// 本体自治：实例根就是本体可执行文件所在目录，全部资产按本体同级相对位置生成；
	// 取消任何外部定位参数（EUCLI_BOX_PROGRAM_ROOT 已退役）。
	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate eucli-box executable: %w", err)
	}
	instanceRoot := filepath.Dir(filepath.Clean(executablePath))
	programsRoot := filepath.Join(instanceRoot, "programs")
	toolBodiesRoot := filepath.Join(programsRoot, "ai-tools")
	pluginSourceDir := filepath.Join(programsRoot, "system-plugins")
	programsDirs := []string{programsRoot, toolBodiesRoot, pluginSourceDir, filepath.Join(programsRoot, "local-store")}
	for _, directory := range programsDirs {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("建立程序区 %s 失败：%w", directory, err)
		}
	}
	officialDoer := boxOfficialHTTPDoer{network: networkSystem}
	officialChecker, err := releasecheck.New(releasecheck.Config{
		Client:       officialDoer,
		IndexBase:    strings.TrimSpace(os.Getenv("EUCLI_BOX_RELEASE_INDEX_BASE")),
		DownloadBase: strings.TrimSpace(os.Getenv("EUCLI_BOX_RELEASE_DOWNLOAD_BASE")),
	})
	if err != nil {
		return fmt.Errorf("create official candidate checker: %w", err)
	}

	dataDir := envOrDefault("EUCLI_BOX_DATA_DIR", filepath.Join(instanceRoot, "data"))
	dataLock, err := localrun.AcquireDataLock(dataDir)
	if err != nil {
		return err
	}
	defer dataLock.Release()

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
	toolProgramRoot := toolBodiesRoot
	storageSystem, err := datastorage.NewSystem(datastorage.Config{RootDir: dataDir, ToolBodiesRoot: toolBodiesRoot})
	if err != nil {
		return fmt.Errorf("start data storage system: %w", err)
	}
	if err := storageSystem.Initialize(ctx); err != nil {
		return fmt.Errorf("initialize data storage system: %w", err)
	}
	log.Printf("[3/13] data-storage-system     ✓  (%s)", dataDir)

	localStoreDir := envOrDefault("EUCLI_BOX_LOCAL_STORE", filepath.Join(programsRoot, "local-store"))
	localCandidateReader, err := releasecheck.NewLocalSourceReader(localStoreDir)
	if err != nil {
		return fmt.Errorf("本地商店读取器不可用：%w", err)
	}
	initialSource := installsource.KindOfficial
	loaded, loadErr := storageSystem.LoadInstallSource(ctx)
	if loadErr != nil {
		if !errors.Is(loadErr, os.ErrNotExist) {
			return fmt.Errorf("读取安装来源配置失败：%w", loadErr)
		}
	} else {
		initialSource = loaded
	}
	sourceState, err := installsource.NewState(initialSource, storageSystem)
	if err != nil {
		return err
	}
	toolCandidates, err := installsource.NewCandidateSelector(sourceState.Current, officialChecker, localCandidateReader)
	if err != nil {
		return err
	}
	log.Printf("[2.6/13] candidate reader %s (install-source: %s)", programStatusLabel(programsRoot), sourceState.Current())

	requestRecordSystem, err := requestrecord.NewSystem(networkSystem, storageSystem)
	if err != nil {
		return fmt.Errorf("start request record system: %w", err)
	}
	log.Printf("[3.5/13] request-record-system    ✓")

	providerSystem, err := modelprovider.NewSystem(modelprovider.Config{}, requestRecordSystem, storageSystem)
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

	toolSystem, err := toolcalling.NewSystem(toolcalling.Config{BoxVersion: boxRelease.Version, ProgramRoot: toolProgramRoot, Candidates: toolCandidates, HTTPClient: officialDoer}, permissionSystem, storageSystem)
	if err != nil {
		return fmt.Errorf("start tool calling system: %w", err)
	}
	if err := toolSystem.StartWarmup(ctx); err != nil {
		return fmt.Errorf("start tool warmup scheduler: %w", err)
	}
	log.Printf("[7/13] tool-calling-system     ✓")

	pluginDataDir := datapaths.SystemPluginsDataDir(dataDir)
	systemPluginSystem, err := systemplugin.NewSystem(systemplugin.Config{
		SourceDir:   pluginSourceDir,
		DataDir:     pluginDataDir,
		BoxVersion:  boxRelease.Version,
		ProgramRoot: pluginSourceDir,
		Candidates:  toolCandidates,
		HTTPClient:  officialDoer,
		OnEvent: func(pluginID string, event string, _ map[string]any) {
			log.Printf("[system-plugin:%s] event %s", pluginID, event)
		},
	})
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

	releaseSourceSystem, err := releasesourcesystem.NewSystemWithChecker(releasesourcesystem.Config{BoxVersion: boxRelease.Version, CurrentSource: sourceState.Current, LocalSource: localCandidateReader}, officialChecker, toolSystem, systemPluginSystem)
	if err != nil {
		return fmt.Errorf("start release source system: %w", err)
	}
	log.Printf("[12/13] release-source-system   ✓")

	accessSystem, err := accesssystem.NewSystem(dataDir)
	if err != nil {
		return fmt.Errorf("start access system: %w", err)
	}
	log.Printf("[12.5/13] access-system            ✓")

	profile, err := serviceprofile.Ensure(dataDir)
	if err != nil {
		return fmt.Errorf("准备启动配置画像失败：%w", err)
	}
	gatewayConfig := gateway.Config{Addr: profile.ListenAddr(), Key: profile.Key, BoxVersion: boxRelease.Version, Access: accessSystem, InstallSource: sourceState}
	gatewaySystem, err := gateway.NewSystem(gatewayConfig, runtimeSystem, roleSystem, storageSystem, storageSystem, providerSystem, toolSystem, storageSystem, storageSystem, storageSystem, placeholderSystem, systemPluginSystem, assistSystem, releaseSourceSystem, requestRecordSystem)
	if err != nil {
		return fmt.Errorf("start gateway system: %w", err)
	}
	accessSystem.SetHandler(gatewaySystem.LongTermHandler())
	log.Printf("[13/13] gateway-system         ✓ (key: active)")

	if err := gatewaySystem.Start(ctx); err != nil {
		return fmt.Errorf("start gateway listener: %w", err)
	}
	accessSystem.SetLocalEntrypointPort(profile.Port)
	if err := migrationSession.Complete(ctx); err != nil {
		return fmt.Errorf("数据迁移收尾失败：%w", err)
	}
	migrationCompleted = true
	accessSystem.Start(ctx)
	log.Printf("eucli-box v%s is ready — listening on %s", boxRelease.Version, gatewaySystem.Endpoint())

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	accessSystem.Shutdown(shutdownCtx)
	if err := gatewaySystem.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown gateway system: %w", err)
	}
	if err := toolSystem.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown tool calling system: %w", err)
	}
	if err := systemPluginSystem.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown system plugin system: %w", err)
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

func programStatusLabel(programsRoot string) string {
	if programsRoot == "" {
		return "(instance root missing)"
	}
	return "✓  (" + programsRoot + ")"
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
