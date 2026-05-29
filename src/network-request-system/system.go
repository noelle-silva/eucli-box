package networkrequest

import (
	"context"
	"time"

	"eucli-box/pkg/types"
)

type System interface {
	Do(ctx context.Context, req types.HTTPRequest) (types.HTTPResponse, error)
}

type Config struct {
	DefaultTimeout time.Duration
	MaxTimeout     time.Duration
	UserAgent      string
}

type system struct {
	client *client
	config Config
}

func NewSystem(config Config) (System, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}
	return &system{client: newClient(), config: normalized}, nil
}

func (s *system) Do(ctx context.Context, req types.HTTPRequest) (types.HTTPResponse, error) {
	prepared, err := buildRequest(ctx, req, s.config)
	if err != nil {
		return types.HTTPResponse{}, err
	}
	return s.client.do(prepared)
}

func normalizeConfig(config Config) (Config, error) {
	if config.DefaultTimeout < 0 {
		return Config{}, invalidRequest("default timeout cannot be negative", nil)
	}
	if config.DefaultTimeout == 0 {
		config.DefaultTimeout = 30 * time.Second
	}
	if config.MaxTimeout <= 0 {
		return Config{}, invalidRequest("max timeout must be positive", nil)
	}
	if config.DefaultTimeout > config.MaxTimeout {
		return Config{}, invalidRequest("default timeout cannot exceed max timeout", nil)
	}
	if config.UserAgent == "" {
		config.UserAgent = "eucli-box/1.0"
	}
	return config, nil
}
