package gateway

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
)

func (s *system) Start(ctx context.Context) error {
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

func (s *system) Shutdown(ctx context.Context) error {
	if err := s.server.Shutdown(ctx); err != nil {
		return gatewayServerFailed("failed to shutdown gateway server", err)
	}
	s.closeConnections()
	return nil
}
