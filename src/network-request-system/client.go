package networkrequest

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

type client struct {
	httpClient *http.Client
}

type preparedRequest struct {
	request *http.Request
	started int64
	cancel  context.CancelFunc
	timeout time.Duration
}

func newClient() *client {
	return &client{httpClient: &http.Client{}}
}

func (c *client) do(prepared preparedRequest) (types.HTTPResponse, error) {
	monitor := startTotalTimeoutMonitor(prepared.timeout, prepared.cancel)
	defer func() {
		monitor.stop()
		prepared.cancel()
	}()
	resp, err := c.httpClient.Do(prepared.request)
	if err != nil {
		if isRequestTimeout(err, prepared.request.Context(), monitor) {
			return types.HTTPResponse{}, requestTimeout("http request timed out", err)
		}
		return types.HTTPResponse{}, classifyRequestError(err)
	}
	defer resp.Body.Close()
	return normalizeResponse(resp, prepared.started, monitor)
}

func (c *client) doStream(prepared preparedRequest, onChunk types.HTTPStreamHandler) (types.HTTPResponse, error) {
	monitor := startIdleTimeoutMonitor(prepared.timeout, prepared.cancel)
	defer func() {
		monitor.stop()
		prepared.cancel()
	}()
	resp, err := c.httpClient.Do(prepared.request)
	if err != nil {
		if isRequestTimeout(err, prepared.request.Context(), monitor) {
			return types.HTTPResponse{}, requestTimeout("http request timed out", err)
		}
		return types.HTTPResponse{}, classifyRequestError(err)
	}
	monitor.touch()
	defer resp.Body.Close()
	return normalizeStreamResponse(resp, prepared.started, onChunk, monitor)
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func isRequestTimeout(err error, ctx context.Context, monitor *timeoutMonitor) bool {
	if monitor != nil && monitor.timedOut() {
		return true
	}
	return isTimeout(err) || errors.Is(ctx.Err(), context.DeadlineExceeded)
}

func classifyRequestError(err error) error {
	if err == nil {
		return nil
	}
	var urlErr *url.Error
	if !errors.As(err, &urlErr) || urlErr.Err == nil {
		return requestFailed("http request failed: "+err.Error(), err)
	}
	if errors.Is(urlErr.Err, context.DeadlineExceeded) {
		return requestTimeout("http request timed out", urlErr)
	}
	var dnsErr *net.DNSError
	if errors.As(urlErr.Err, &dnsErr) {
		return dnsFailed(fmt.Sprintf("DNS lookup failed for %s: %s", dnsErr.Name, dnsErr.Err), urlErr)
	}
	var opErr *net.OpError
	if errors.As(urlErr.Err, &opErr) {
		if opErr.Op == "dial" {
			message := fmt.Sprintf("connection to %s failed: %s", opErr.Addr, opErr.Err)
			if isConnectionRefusedMessage(message) {
				return connectionRefused(message, urlErr)
			}
			return connectionFailed(message, urlErr)
		}
		op := strings.ToLower(opErr.Op)
		if strings.Contains(op, "tls") || strings.Contains(op, "handshake") {
			return tlsFailed(fmt.Sprintf("TLS handshake with %s failed: %s", opErr.Addr, opErr.Err), urlErr)
		}
		if opErr.Op == "read" {
			return connectionLost(fmt.Sprintf("connection lost while reading from %s: %s", opErr.Addr, opErr.Err), urlErr)
		}
	}
	message := urlErr.Err.Error()
	lowerMessage := strings.ToLower(message)
	if isConnectionRefusedMessage(message) {
		return connectionRefused("connection refused: "+message, urlErr)
	}
	if strings.Contains(lowerMessage, "no such host") {
		return dnsFailed("DNS lookup failed: "+message, urlErr)
	}
	if strings.Contains(lowerMessage, "connection reset") || strings.Contains(message, "EOF") {
		return connectionLost("connection lost: "+message, urlErr)
	}
	if strings.Contains(lowerMessage, "tls") || strings.Contains(lowerMessage, "certificate") {
		return tlsFailed("TLS handshake failed: "+message, urlErr)
	}
	return requestFailed("http request failed: "+message, urlErr)
}

func isConnectionRefusedMessage(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "connection refused") || strings.Contains(lower, "actively refused")
}
