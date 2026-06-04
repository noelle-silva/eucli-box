package networkrequest

import (
	"context"
	"errors"
	"net"
	"net/http"
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
		return types.HTTPResponse{}, requestFailed("http request failed", err)
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
		return types.HTTPResponse{}, requestFailed("http request failed", err)
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
