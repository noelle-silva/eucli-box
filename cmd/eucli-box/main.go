package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	agentruntime "eucli-box/src/agent-runtime-system"
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

	networkSystem, err := networkrequest.NewSystem(networkrequest.Config{})
	if err != nil {
		return fmt.Errorf("start network request system: %w", err)
	}

	storageSystem, err := datastorage.NewSystem(datastorage.Config{RootDir: envOrDefault("EUCLI_BOX_DATA_DIR", "data")})
	if err != nil {
		return fmt.Errorf("start data storage system: %w", err)
	}
	if err := storageSystem.Initialize(ctx); err != nil {
		return fmt.Errorf("initialize data storage system: %w", err)
	}

	providerSystem, err := modelprovider.NewSystem(modelprovider.Config{}, networkSystem, storageSystem)
	if err != nil {
		return fmt.Errorf("start model provider system: %w", err)
	}

	roleSystem, err := roleprompt.NewSystem(roleprompt.Config{}, storageSystem, providerSystem)
	if err != nil {
		return fmt.Errorf("start role prompt system: %w", err)
	}

	permissionSystem, err := permission.NewSystem(permission.Config{}, roleSystem)
	if err != nil {
		return fmt.Errorf("start permission system: %w", err)
	}

	toolSystem, err := toolcalling.NewSystem(toolcalling.Config{}, permissionSystem, storageSystem)
	if err != nil {
		return fmt.Errorf("start tool calling system: %w", err)
	}

	runtimeSystem, err := agentruntime.NewSystem(agentruntime.Config{}, storageSystem, roleSystem, providerSystem, toolSystem)
	if err != nil {
		return fmt.Errorf("start agent runtime system: %w", err)
	}

	gatewaySystem, err := gateway.NewSystem(gateway.Config{Addr: envOrDefault("EUCLI_BOX_ADDR", "127.0.0.1:8765")}, runtimeSystem, roleSystem, providerSystem, toolSystem)
	if err != nil {
		return fmt.Errorf("start gateway system: %w", err)
	}

	if err := gatewaySystem.Start(ctx); err != nil {
		return fmt.Errorf("start gateway listener: %w", err)
	}
	log.Printf("eucli-box listening on %s", envOrDefault("EUCLI_BOX_ADDR", "127.0.0.1:8765"))

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
