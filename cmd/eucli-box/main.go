package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"eucli-box/pkg/types"
	agentruntime "eucli-box/src/agent-runtime-system"
	aiassist "eucli-box/src/ai-assist-system"
	datastorage "eucli-box/src/data-storage-system"
	gateway "eucli-box/src/gateway-system"
	modelprovider "eucli-box/src/model-provider-system"
	networkrequest "eucli-box/src/network-request-system"
	permission "eucli-box/src/permission-system"
	roleprompt "eucli-box/src/role-prompt-system"
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

	networkSystem, err := networkrequest.NewSystem(networkrequest.Config{MaxTimeout: time.Duration(types.ModelRequestCompletionTimeoutMaxMs) * time.Millisecond})
	if err != nil {
		return fmt.Errorf("start network request system: %w", err)
	}
	log.Printf("[1/8] network-request-system  ✓")

	dataDir := envOrDefault("EUCLI_BOX_DATA_DIR", "data")
	storageSystem, err := datastorage.NewSystem(datastorage.Config{RootDir: dataDir})
	if err != nil {
		return fmt.Errorf("start data storage system: %w", err)
	}
	if err := storageSystem.Initialize(ctx); err != nil {
		return fmt.Errorf("initialize data storage system: %w", err)
	}
	log.Printf("[2/8] data-storage-system      ✓  (%s)", dataDir)

	providerSystem, err := modelprovider.NewSystem(modelprovider.Config{}, networkSystem, storageSystem)
	if err != nil {
		return fmt.Errorf("start model provider system: %w", err)
	}
	log.Printf("[3/8] model-provider-system    ✓")

	roleSystem, err := roleprompt.NewSystem(roleprompt.Config{}, storageSystem, providerSystem)
	if err != nil {
		return fmt.Errorf("start role prompt system: %w", err)
	}
	log.Printf("[4/8] role-prompt-system       ✓")

	permissionSystem, err := permission.NewSystem(permission.Config{}, roleSystem)
	if err != nil {
		return fmt.Errorf("start permission system: %w", err)
	}
	log.Printf("[5/8] permission-system        ✓")

	toolSystem, err := toolcalling.NewSystem(toolcalling.Config{}, permissionSystem, storageSystem)
	if err != nil {
		return fmt.Errorf("start tool calling system: %w", err)
	}
	log.Printf("[6/8] tool-calling-system      ✓")

	runtimeSystem, err := agentruntime.NewSystem(agentruntime.Config{}, storageSystem, roleSystem, providerSystem, toolSystem)
	if err != nil {
		return fmt.Errorf("start agent runtime system: %w", err)
	}
	log.Printf("[7/9] agent-runtime-system     ✓")

	assistSystem, err := aiassist.NewSystem(aiassist.Config{}, storageSystem, providerSystem)
	if err != nil {
		return fmt.Errorf("start ai assist system: %w", err)
	}
	log.Printf("[8/9] ai-assist-system         ✓")

	busyKey := ""
	if readBoxKey(dataDir) != "" {
		busyKey = " (key: active)"
	}
	gatewaySystem, err := gateway.NewSystem(gateway.Config{Addr: envOrDefault("EUCLI_BOX_ADDR", "127.0.0.1:8765"), Key: readBoxKey(dataDir)}, runtimeSystem, roleSystem, storageSystem, storageSystem, providerSystem, toolSystem, storageSystem, storageSystem, storageSystem, assistSystem)
	if err != nil {
		return fmt.Errorf("start gateway system: %w", err)
	}
	log.Printf("[9/9] gateway-system           ✓%s", busyKey)

	log.Printf("eucli-box is starting on %s ...", envOrDefault("EUCLI_BOX_ADDR", "127.0.0.1:8765"))
	if err := gatewaySystem.Start(ctx); err != nil {
		return fmt.Errorf("start gateway listener: %w", err)
	}
	log.Printf("eucli-box is ready — listening on %s", envOrDefault("EUCLI_BOX_ADDR", "127.0.0.1:8765"))

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := gatewaySystem.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown gateway system: %w", err)
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
	keyFile := filepath.Join(dataDir, "meta", "box.key")
	payload, err := os.ReadFile(keyFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(payload))
}
