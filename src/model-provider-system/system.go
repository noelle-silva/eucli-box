package modelprovider

import (
	"context"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

type System interface {
	SaveProvider(ctx context.Context, provider types.Provider) error
	LoadProvider(ctx context.Context, providerID string) (types.Provider, error)
	ListProviders(ctx context.Context) ([]types.ProviderSummary, error)
	DeleteProvider(ctx context.Context, providerID string) error

	RefreshModels(ctx context.Context, providerID string) ([]types.ModelInfo, error)
	ResolveModel(ctx context.Context, coordinate types.ModelCoordinate) (types.Provider, types.ModelInfo, error)
	Complete(ctx context.Context, request types.ModelRequest) (types.ModelResponse, error)
}

type NetworkSystem interface {
	Do(ctx context.Context, req types.HTTPRequest) (types.HTTPResponse, error)
}

type StorageSystem interface {
	SaveProvider(ctx context.Context, provider types.Provider) error
	LoadProvider(ctx context.Context, providerID string) (types.Provider, error)
	ListProviders(ctx context.Context) ([]types.ProviderSummary, error)
	DeleteProvider(ctx context.Context, providerID string) error
}

type Config struct {
	RequestTimeout time.Duration
}

type system struct {
	config  Config
	network NetworkSystem
	storage StorageSystem
}

func NewSystem(config Config, network NetworkSystem, storage StorageSystem) (System, error) {
	if network == nil {
		return nil, providerInvalid("network system dependency is required", nil)
	}
	if storage == nil {
		return nil, providerInvalid("storage system dependency is required", nil)
	}
	if config.RequestTimeout < 0 {
		return nil, providerInvalid("request timeout cannot be negative", nil)
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 60 * time.Second
	}
	return &system{config: config, network: network, storage: storage}, nil
}

func normalizeBaseURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}
