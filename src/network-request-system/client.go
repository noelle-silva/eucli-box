package networkrequest

import (
	"context"
	"errors"
	"net"
	"net/http"

	"eucli-box/pkg/types"
)

type client struct {
	httpClient *http.Client
}

type preparedRequest struct {
	request *http.Request
	started int64
	cancel  context.CancelFunc
}

func newClient() *client {
	return &client{httpClient: &http.Client{}}
}

func (c *client) do(prepared preparedRequest) (types.HTTPResponse, error) {
	defer prepared.cancel()
	resp, err := c.httpClient.Do(prepared.request)
	if err != nil {
		if isTimeout(err) || errors.Is(prepared.request.Context().Err(), context.DeadlineExceeded) {
			return types.HTTPResponse{}, requestTimeout("http request timed out", err)
		}
		return types.HTTPResponse{}, requestFailed("http request failed", err)
	}
	defer resp.Body.Close()
	return normalizeResponse(resp, prepared.started)
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
