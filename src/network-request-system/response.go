package networkrequest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"eucli-box/pkg/types"
)

func normalizeResponse(resp *http.Response, started int64, monitor *timeoutMonitor) (types.HTTPResponse, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if monitor.timedOut() || errors.Is(err, context.DeadlineExceeded) || errors.Is(resp.Request.Context().Err(), context.DeadlineExceeded) {
			return types.HTTPResponse{}, requestTimeout("http request timed out", err)
		}
		return types.HTTPResponse{}, requestFailed("failed to read http response body", err)
	}
	return types.HTTPResponse{
		StatusCode: resp.StatusCode,
		Headers:    cloneHeaders(resp.Header),
		Body:       body,
		Duration:   time.Since(time.Unix(0, started)),
	}, nil
}

func normalizeStreamResponse(resp *http.Response, started int64, onChunk types.HTTPStreamHandler, monitor *timeoutMonitor) (types.HTTPResponse, error) {
	var body bytes.Buffer
	buffer := make([]byte, 32*1024)
	streamChunks := onChunk != nil && resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			monitor.touch()
			data := append([]byte(nil), buffer[:n]...)
			_, _ = body.Write(data)
			if streamChunks {
				if handlerErr := onChunk(types.HTTPStreamChunk{Data: data, Duration: time.Since(time.Unix(0, started))}); handlerErr != nil {
					return types.HTTPResponse{}, handlerErr
				}
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if monitor.timedOut() || errors.Is(err, context.DeadlineExceeded) || errors.Is(resp.Request.Context().Err(), context.DeadlineExceeded) {
				return types.HTTPResponse{}, requestTimeout("http request timed out", err)
			}
			return types.HTTPResponse{}, requestFailed("failed to read http response body", err)
		}
	}
	return types.HTTPResponse{StatusCode: resp.StatusCode, Headers: cloneHeaders(resp.Header), Body: body.Bytes(), Duration: time.Since(time.Unix(0, started))}, nil
}

func cloneHeaders(headers http.Header) map[string][]string {
	cloned := make(map[string][]string, len(headers))
	for key, values := range headers {
		clonedValues := make([]string, len(values))
		copy(clonedValues, values)
		cloned[key] = clonedValues
	}
	return cloned
}
