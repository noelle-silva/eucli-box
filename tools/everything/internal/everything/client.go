package everything

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"eucli-box/tools/everything/internal/types"
	networkrequest "eucli-box/tools/everything/internal/networkrequest"
)

// rpcClient 是 Everything 应用开放接口的最小客户端：
// 以访问钥匙发起 JSON-RPC 请求，并统一归一响应与错误。
type rpcClient struct {
	network        networkrequest.System
	endpoint       string
	key            string
	requestTimeout time.Duration
}

func newRPCClient(endpoint string, key string, requestTimeout time.Duration) (*rpcClient, error) {
	network, err := networkrequest.NewSystem(networkrequest.Config{UserAgent: "eucli-box-everything/1.0"})
	if err != nil {
		return nil, fmt.Errorf("初始化网络组件失败：%w", err)
	}
	return &rpcClient{network: network, endpoint: endpoint, key: key, requestTimeout: requestTimeout}, nil
}

// call 发起一次开放接口请求：成功返回 result 原文；失败返回可读的错误。
func (c *rpcClient) call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	payload, err := json.Marshal(map[string]any{"method": method, "params": params})
	if err != nil {
		return nil, fmt.Errorf("构造请求失败：%w", err)
	}
	response, err := c.network.Do(ctx, types.HTTPRequest{
		Method:   http.MethodPost,
		URL:      c.endpoint + "/rpc",
		Headers:  map[string]string{"Authorization": "Bearer " + c.key},
		BodyKind: types.HTTPBodyJSON,
		Body:     payload,
		Timeout:  c.requestTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("无法连接 Everything 应用（%s）：%w；请确认 Everything 应用正在运行，且访问地址与开放端口一致", c.endpoint, err)
	}
	var envelope struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  map[string]any  `json:"error"`
	}
	if err := json.Unmarshal(response.Body, &envelope); err != nil {
		body, _ := truncateRunes(string(response.Body), 200)
		return nil, fmt.Errorf("Everything 应用响应格式无效（HTTP %d）：%s", response.StatusCode, body)
	}
	if !envelope.OK {
		message := errorMessage(envelope.Error)
		if message == "" {
			message = fmt.Sprintf("Everything 应用请求失败（HTTP %d）", response.StatusCode)
		}
		// 后端错误信封带机器可读错误码：原样透传给调用方。
		if code, _ := envelope.Error["code"].(string); strings.TrimSpace(code) != "" {
			return nil, coded(strings.TrimSpace(code), "%s", message)
		}
		return nil, errors.New(message)
	}
	return envelope.Result, nil
}

// errorMessage 提取错误信封中的消息文本。
func errorMessage(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	message, _ := payload["message"].(string)
	return strings.TrimSpace(message)
}
