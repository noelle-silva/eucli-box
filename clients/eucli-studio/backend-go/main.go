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
	source, checker, devBoxRoot, err := resolveLocalBoxSource()
	if err != nil {
		return err
	}
	if source == nil {
		officialChecker, err := releasecheck.New(releasecheck.Config{})
		if err != nil {
			return err
		}
		checker = officialChecker
		source = &officialArtifactSource{checker: officialChecker}
	}
	svc, err := newService(store, release, hub, source, checker, devBoxRoot)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	svc.startStandaloneReleaseCheck(ctx)
	go newEventBridge(svc, hub).run(ctx)
	serverErr := newDirectServer(token, svc, hub).listenAndServe(ctx)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if shutdownErr := svc.shutdownLocalBox(shutdownCtx); shutdownErr != nil && serverErr == nil {
		return shutdownErr
	}
	return serverErr
}

// resolveLocalBoxSource 根据开发体验入口的环境变量显式建立成品来源。
// 没有开发来源标记时返回 nil，由调用方建立官方来源；
// 有开发来源标记时即使资料缺失也返回开发来源，让客户端直接报告开发成品不可用，
// 绝不回退到读取官方旧正式版。
func resolveLocalBoxSource() (localBoxArtifactSource, releaseChecker, string, error) {
	if strings.TrimSpace(os.Getenv(devSourceEnvironment)) != devSourceEnabled {
		return nil, nil, "", nil
	}
	return newDevelopmentArtifactSource(os.Getenv(devManifestEnvironment), os.Getenv(devArchiveEnvironment)), nil, strings.TrimSpace(os.Getenv(devBoxRootEnvironment)), nil
}
