package hypercortexreader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"eucli-box/pkg/types"
	networkrequest "eucli-box/src/network-request-system"
)

// rpcClient 是 HyperCortex 外部访问接口的最小客户端：
// 以仓库条目的访问钥匙发起 JSON-RPC 请求，并统一归一响应与错误。
type rpcClient struct {
	network  networkrequest.System
	endpoint string
	key      string
}

func newRPCClient(endpoint string, key string) (*rpcClient, error) {
	network, err := networkrequest.NewSystem(networkrequest.Config{UserAgent: "eucli-box-hypercortex-reader/1.0"})
	if err != nil {
		return nil, fmt.Errorf("初始化网络组件失败：%w", err)
	}
	return &rpcClient{network: network, endpoint: endpoint, key: key}, nil
}

// call 发起一次外部访问请求：成功返回 result 原文；失败返回可读的错误。
// 请求参数不携带仓库作用域：仓库身份由访问钥匙自身决定，工具无权也无法越权指定。
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
	})
	if err != nil {
		return nil, fmt.Errorf("无法连接 HyperCortex 外部访问服务（%s）：%w；请确认 HyperCortex 正在运行，且访问地址与开放端口一致", c.endpoint, err)
	}
	var envelope struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  map[string]any  `json:"error"`
	}
	if err := json.Unmarshal(response.Body, &envelope); err != nil {
		body, _ := truncateRunes(string(response.Body), 200)
		return nil, fmt.Errorf("HyperCortex 响应格式无效（HTTP %d）：%s", response.StatusCode, body)
	}
	if !envelope.OK {
		message := errorMessage(envelope.Error)
		if message == "" {
			message = fmt.Sprintf("HyperCortex 请求失败（HTTP %d）", response.StatusCode)
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
