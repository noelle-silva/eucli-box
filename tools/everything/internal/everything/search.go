package everything

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// searchEverything 通过 Everything 应用开放接口执行一次搜索，返回结构化结果。
func searchEverything(ctx context.Context, client *rpcClient, request searchRequest) (searchResultPayload, error) {
	params := map[string]any{"query": request.Query}
	if request.MaxResults > 0 {
		params["limit"] = request.MaxResults
	}
	if request.ScopePath != "" {
		params["scopePath"] = request.ScopePath
	}
	raw, err := client.call(ctx, "everything.search", params)
	if err != nil {
		return searchResultPayload{}, err
	}
	var payload searchResultPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return searchResultPayload{}, fmt.Errorf("解析搜索结果失败：%w", err)
	}
	return payload, nil
}

// requestTimeout 计算本次请求时限：调用参数优先，否则用随包缺省。
func requestTimeout(request searchRequest, config Config) time.Duration {
	if request.TimeoutMs > 0 {
		return time.Duration(request.TimeoutMs) * time.Millisecond
	}
	return time.Duration(config.Limits.DefaultRequestTimeoutMs) * time.Millisecond
}
