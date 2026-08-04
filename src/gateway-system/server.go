package gateway

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
)

func (s *system) Start(ctx context.Context) error {
	if s.config.LocalRun {
		return gatewayServerFailed("普通启动不能用于受托模式", nil)
	}
	listener, err := net.Listen("tcp", s.config.Addr)
	if err != nil {
		return gatewayServerFailed("failed to listen gateway address", err)
	}
	go func() {
		err := s.server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("gateway server error: %v", err)
		}
	}()
	return nil
}

func (s *system) StartLocal(ctx context.Context) (LocalStartResult, error) {
	if !s.config.LocalRun {
		return LocalStartResult{}, gatewayServerFailed("当前不是受托模式", nil)
	}
	if s.config.Addr != "127.0.0.1:0" {
		return LocalStartResult{}, gatewayServerFailed("受托模式地址必须为 127.0.0.1:0", nil)
	}
	listener, err := net.Listen("tcp", s.config.Addr)
	if err != nil {
		return LocalStartResult{}, gatewayServerFailed("failed to listen local gateway address", err)
	}
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok || address.IP == nil || !address.IP.IsLoopback() || address.Port <= 0 {
		_ = listener.Close()
		return LocalStartResult{}, gatewayServerFailed("local gateway did not return a loopback address", nil)
	}
	s.endpoint = fmt.Sprintf("http://127.0.0.1:%d", address.Port)
	go func() {
		err := s.server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("gateway local server error: %v", err)
		}
	}()
	return LocalStartResult{Endpoint: s.endpoint}, nil
}

func (s *system) Shutdown(ctx context.Context) error {
	if err := s.server.Shutdown(ctx); err != nil {
		return gatewayServerFailed("failed to shutdown gateway server", err)
	}
	s.closeConnections()
	return nil
}
