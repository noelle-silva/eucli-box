package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"eucli-box/internal/boxrelease"
	"eucli-box/pkg/datapaths"
	"eucli-box/pkg/installsource"
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

	toolSystem, err := toolcalling.NewSystem(toolcalling.Config{BoxVersion: boxRelease.Version, ProgramRoot: toolProgramRoot, Candidates: toolCandidates, HTTPClient: officialDoer}, permissionSystem, storageSystem)
	if err != nil {
		return fmt.Errorf("start tool calling system: %w", err)
	}
	log.Printf("[7/13] tool-calling-system     ✓")

	pluginDataDir := datapaths.SystemPluginsDataDir(dataDir)
	systemPluginSystem, err := systemplugin.NewSystem(systemplugin.Config{SourceDir: pluginSourceDir, DataDir: pluginDataDir, BoxVersion: boxRelease.Version, ProgramRoot: pluginSourceDir, Candidates: toolCandidates, HTTPClient: officialDoer})
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

	busyKey := ""
	boxKey := ensureBoxKey(dataDir)
	if boxKey != "" {
		busyKey = " (key: active)"
	}
	if err := ensurePortFile(dataDir); err != nil {
		return err
	}
	listenAddr, err := resolveListenAddr(dataDir)
	if err != nil {
		return err
	}
	gatewayConfig := gateway.Config{Addr: listenAddr, Key: boxKey, BoxVersion: boxRelease.Version, Access: accessSystem, InstallSource: sourceState}
	gatewaySystem, err := gateway.NewSystem(gatewayConfig, runtimeSystem, roleSystem, storageSystem, storageSystem, providerSystem, toolSystem, storageSystem, storageSystem, storageSystem, placeholderSystem, systemPluginSystem, assistSystem, releaseSourceSystem)
	if err != nil {
		return fmt.Errorf("start gateway system: %w", err)
	}
	accessSystem.SetHandler(gatewaySystem.LongTermHandler())
	log.Printf("[13/13] gateway-system         ✓%s", busyKey)

	if err := gatewaySystem.Start(ctx); err != nil {
		return fmt.Errorf("start gateway listener: %w", err)
	}
	if entrypoint := entrypointPortFromAddr(gatewayConfig.Addr); entrypoint > 0 {
		accessSystem.SetLocalEntrypointPort(entrypoint)
	}
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

func readBoxKey(dataDir string) string {
	if envKey := strings.TrimSpace(os.Getenv("EUCLI_BOX_KEY")); envKey != "" {
		return envKey
	}
	keyFile := datapaths.BoxKeyFile(dataDir)
	payload, err := os.ReadFile(keyFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(payload))
}

// ensureBoxKey 保证数据房存有访问钥匙：首次启动时自行生成并记录，以后复用；
// 不依赖任何外部注入。
func ensureBoxKey(dataDir string) string {
	existing := readBoxKey(dataDir)
	if existing != "" {
		return existing
	}
	buffer := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, buffer); err != nil {
		log.Printf("生成访问钥匙失败：%v", err)
		return ""
	}
	key := hex.EncodeToString(buffer)
	metaDir := datapaths.MetaDir(dataDir)
	if err := os.MkdirAll(metaDir, 0o700); err != nil {
		log.Printf("生成访问钥匙失败：%v", err)
		return ""
	}
	if err := os.WriteFile(filepath.Join(metaDir, "box.key"), []byte(key+"\n"), 0o600); err != nil {
		log.Printf("生成访问钥匙失败：%v", err)
		return ""
	}
	return key
}

// 端口配置事实：gatewayHost 是业务端监听的内置宿主地址，
// defaultGatewayAddr 是 port 取值为 "default" 时的完整内置监听地址。
const (
	gatewayHost        = "127.0.0.1"
	defaultGatewayAddr = gatewayHost + ":8765"
	portDefaultValue   = "default"
	portMinValue       = 1
	portMaxValue       = 65535
)

// portFileDocument 是端口配置文件的写入形态。
type portFileDocument struct {
	Port string `json:"port"`
}

// ensurePortFile 保证数据房存有端口配置文件：首次启动时写入默认内容，已存在则绝不改动。
func ensurePortFile(dataDir string) error {
	portFile := datapaths.PortFile(dataDir)
	_, err := os.Stat(portFile)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("读取端口配置失败：%w", err)
	}
	payload, err := json.MarshalIndent(portFileDocument{Port: portDefaultValue}, "", "  ")
	if err != nil {
		return fmt.Errorf("生成端口配置失败：%w", err)
	}
	if err := os.MkdirAll(datapaths.MetaDir(dataDir), 0o700); err != nil {
		return fmt.Errorf("生成端口配置失败：%w", err)
	}
	if err := os.WriteFile(portFile, append(payload, '\n'), 0o600); err != nil {
		return fmt.Errorf("生成端口配置失败：%w", err)
	}
	return nil
}

// readPortFile 读取并校验端口配置文件：返回 nil 表示配置为 "default"（使用内置默认地址），
// 否则返回 1-65535 的端口号；任何非法取值都直接报错（快速失败，不静默回退）。
func readPortFile(dataDir string) (*int, error) {
	portFile := datapaths.PortFile(dataDir)
	payload, err := os.ReadFile(portFile)
	if err != nil {
		return nil, fmt.Errorf("读取端口配置失败：%w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("端口配置 %s 不是有效 JSON：%w", portFile, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("端口配置 %s 不是有效 JSON：包含多余内容", portFile)
	}
	value, exists := document["port"]
	if !exists {
		return nil, fmt.Errorf("端口配置 %s 缺少 port 字段", portFile)
	}
	text, err := portValueText(value)
	if err != nil {
		return nil, fmt.Errorf("端口配置 %s 的 port 取值无效：%w", portFile, err)
	}
	if text == portDefaultValue {
		return nil, nil
	}
	port, err := strconv.Atoi(text)
	if err != nil {
		return nil, fmt.Errorf("端口配置 %s 的 port 取值无效：必须是 %d-%d 之间的整数，实际为 %q", portFile, portMinValue, portMaxValue, text)
	}
	if port < portMinValue || port > portMaxValue {
		return nil, fmt.Errorf("端口配置 %s 的 port 取值无效：必须在 %d-%d 之间，实际为 %d", portFile, portMinValue, portMaxValue, port)
	}
	return &port, nil
}

// portValueText 把 port 字段归一到文本：字符串取去除首尾空白后的内容，JSON 数字取字面量，其余类型拒绝。
func portValueText(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed), nil
	case json.Number:
		return typed.String(), nil
	case nil:
		return "", errors.New("不能为 null")
	case bool:
		return "", errors.New("不能为布尔值")
	case []any:
		return "", errors.New("不能为数组")
	case map[string]any:
		return "", errors.New("不能为对象")
	default:
		return "", fmt.Errorf("类型不受支持：%T", value)
	}
}

// resolveListenAddr 解析最终监听地址，优先级：EUCLI_BOX_ADDR 环境变量 > 配置文件（非 default）> 内置默认。
// 配置文件无论是否被采用都会先读取与校验，环境变量不会掩盖文件坏值。
func resolveListenAddr(dataDir string) (string, error) {
	port, err := readPortFile(dataDir)
	if err != nil {
		return "", err
	}
	if envAddr := strings.TrimSpace(os.Getenv("EUCLI_BOX_ADDR")); envAddr != "" {
		return envAddr, nil
	}
	if port == nil {
		return defaultGatewayAddr, nil
	}
	return gatewayHost + ":" + strconv.Itoa(*port), nil
}

// entrypointPortFromAddr 解析网关监听地址的真实端口；随机端口（0）返回 0。
func entrypointPortFromAddr(addr string) int {
	_, portValue, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(portValue)
	if err != nil || port < 1 {
		return 0
	}
	return port
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
