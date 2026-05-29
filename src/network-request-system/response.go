package networkrequest

import (
	"io"
	"net/http"
	"time"

	"eucli-box/pkg/types"
)

func normalizeResponse(resp *http.Response, started int64) (types.HTTPResponse, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.HTTPResponse{}, requestFailed("failed to read http response body", err)
	}
	return types.HTTPResponse{
		StatusCode: resp.StatusCode,
		Headers:    cloneHeaders(resp.Header),
		Body:       body,
		Duration:   time.Since(time.Unix(0, started).UTC()),
	}, nil
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
