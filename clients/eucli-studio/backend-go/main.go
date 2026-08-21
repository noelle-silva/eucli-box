package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"eucli-box/pkg/releasecheck"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("fatal: %v", err)
		os.Exit(1)
	}
}

func run() error {
	token := strings.TrimSpace(os.Getenv("FW_APP_SESSION_TOKEN"))
	if token == "" {
		return errors.New("eucli-studio backend missing FW_APP_SESSION_TOKEN")
	}
	dataDir := strings.TrimSpace(os.Getenv("FW_APP_DATA_DIR"))
	release, err := loadClientRelease(os.Getenv(clientReleaseEnvironment))
	if err != nil {
		return err
	}
	store, err := newConfigStore(dataDir)
	if err != nil {
		return err
	}
	hub := newEventHub()
	source, devBoxRoot, err := resolveLocalBoxSource(store)
	if err != nil {
		return err
	}
	var checker releaseChecker
	if source == nil {
		officialChecker, err := releasecheck.New(releasecheck.Config{})
		if err != nil {
			return err
		}
		source = &officialArtifactSource{checker: officialChecker}
		checker = officialChecker
	}
	svc, err := newService(store, release, hub, source, checker, devBoxRoot)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go newEventBridge(svc, hub).run(ctx)
	serverErr := newDirectServer(token, svc, hub).listenAndServe(ctx)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if keepBoxRunning, loadErr := keepBoxRunningOnExit(store); loadErr == nil && !keepBoxRunning {
		if shutdownErr := svc.shutdownLocalBox(shutdownCtx); shutdownErr != nil && serverErr == nil {
			return shutdownErr
		}
	}
	return serverErr
}

// keepBoxRunningOnExit 读取客户端设置中的后台运行开关；读取失败时按默认关闭处理。
func keepBoxRunningOnExit(store *configStore) (bool, error) {
	cfg, err := store.load()
	if err != nil {
		return false, err
	}
	return cfg.KeepBoxRunningOnExit, nil
}

// resolveLocalBoxSource 根据开发模式标记与客户端配置决定业务端成品来源。
// 正式模式（无开发标记）返回 nil，由调用方建立官方来源（现状不变）；
// 开发模式按客户端配置选择：development 读取本地成品，official 由调用方建立官方源。
// 有开发来源标记时即使资料缺失也返回开发来源，让客户端直接报告开发成品不可用，
// 绝不回退到读取官方旧正式版。
func resolveLocalBoxSource(store *configStore) (localBoxArtifactSource, string, error) {
	if !devBoxSourceEnabled() {
		return nil, "", nil
	}
	if store.boxSourceKindEffective() == localBoxSourceDevelopment {
		return newDevelopmentArtifactSource(os.Getenv(devManifestEnvironment), os.Getenv(devArchiveEnvironment)), strings.TrimSpace(os.Getenv(devBoxRootEnvironment)), nil
	}
	return nil, strings.TrimSpace(os.Getenv(devBoxRootEnvironment)), nil
}
