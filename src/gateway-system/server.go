package gateway

import (
	"context"
	"errors"
	"net"
	"net/http"
)

func (s *system) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.config.Addr)
	if err != nil {
		return gatewayServerFailed("failed to listen gateway address", err)
	}
	errCh := make(chan error, 1)
	go func() {
		err := s.server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	select {
	case <-ctx.Done():
		_ = s.Shutdown(context.Background())
		return gatewayServerFailed("gateway start cancelled", ctx.Err())
	case err := <-errCh:
		if err != nil {
			return gatewayServerFailed("gateway server failed", err)
		}
		return nil
	default:
		return nil
	}
}

func (s *system) Shutdown(ctx context.Context) error {
	s.closeConnections()
	if err := s.server.Shutdown(ctx); err != nil {
		return gatewayServerFailed("failed to shutdown gateway server", err)
	}
	return nil
}
