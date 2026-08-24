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
	listener, err := net.Listen("tcp", s.config.Addr)
	if err != nil {
		return gatewayServerFailed("failed to listen gateway address", err)
	}
	s.endpoint = endpointFromListener(listener.Addr())
	go func() {
		err := s.server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("gateway server error: %v", err)
		}
	}()
	return nil
}

func (s *system) Endpoint() string {
	return s.endpoint
}

func endpointFromListener(address net.Addr) string {
	tcp, ok := address.(*net.TCPAddr)
	if !ok || tcp.IP == nil || !tcp.IP.IsLoopback() || tcp.Port <= 0 {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%d", tcp.Port)
}

func (s *system) Shutdown(ctx context.Context) error {
	if err := s.server.Shutdown(ctx); err != nil {
		return gatewayServerFailed("failed to shutdown gateway server", err)
	}
	s.closeConnections()
	return nil
}
