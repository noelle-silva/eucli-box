package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

type service struct {
	config *configStore
	eb     *ebClient
}

func newService(config *configStore) *service {
	return &service{config: config, eb: newEBClient(config)}
}

func (s *service) dispatch(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case "aiChat.healthCheck":
		cfg, _ := s.config.load()
		return map[string]any{"version": 1, "status": "ok", "configured": cfg.EucliBoxURL != ""}, nil
	case "studio.bootstrap":
		cfg, _ := s.config.load()
		return map[string]any{"eucliBoxConfigured": cfg.EucliBoxURL != "", "eucliBoxUrl": cfg.EucliBoxURL}, nil
	case "eucli.config.get":
		return s.config.load()
	case "eucli.config.set":
		var cfg clientConfig
		if err := json.Unmarshal(paramsOrEmpty(params), &cfg); err != nil {
			return nil, err
		}
		return s.config.save(cfg)
	case "eucli.eb.request":
		var req ebRequest
		if err := json.Unmarshal(paramsOrEmpty(params), &req); err != nil {
			return nil, err
		}
		return s.eb.request(ctx, req)
	case "aiChat.netRequest":
		return s.handleNetRequest(ctx, params)
	case "aiChat.submitChatCompletion", "aiChat.submitManyChatCompletions", "aiChat.submitRawServiceRequest":
		return nil, newError("EB_MODEL_REQUIRED", fmt.Sprintf("%s 必须通过 e-b runs 接口推进，客户端不再本地运行", method))
	case "aiChat.cancelAssistant", "aiChat.getAssistantRuntime", "aiChat.readAssistantStream", "aiChat.consumeAssistantFinal", "aiChat.resetAssistantRuntime", "aiChat.waitServiceFinal":
		return nil, newError("EB_MODEL_REQUIRED", fmt.Sprintf("%s 必须通过 e-b runs/events 接口读取，客户端不再维护本地运行态", method))
	case "aiChat.storageGet", "aiChat.storageSet", "aiChat.storageRemove":
		return nil, newError("EB_MODEL_REQUIRED", fmt.Sprintf("%s 已禁用：客户端不再维护 e-b 业务事实副本", method))
	case "aiChat.imageRead", "aiChat.imageWrite", "aiChat.imageDelete", "aiChat.imagePick":
		return nil, newError("NOT_IMPLEMENTED", fmt.Sprintf("%s 尚未接入 e-b 附件能力", method))
	default:
		return nil, newError("METHOD_NOT_FOUND", "未知请求："+method)
	}
}

func (s *service) handleNetRequest(ctx context.Context, params json.RawMessage) (any, error) {
	var raw map[string]any
	if err := json.Unmarshal(paramsOrEmpty(params), &raw); err != nil {
		return nil, err
	}
	path, _ := raw["path"].(string)
	if path == "" {
		urlValue, _ := raw["url"].(string)
		path = urlValue
	}
	if path == "" {
		return nil, errors.New("aiChat.netRequest requires path")
	}
	body, _ := json.Marshal(raw["body"])
	query, _ := json.Marshal(raw["query"])
	req := ebRequest{Method: fmt.Sprint(raw["method"]), Path: path, Query: query, Body: body}
	if timeout, ok := raw["timeoutMs"].(float64); ok {
		req.Timeout = int(timeout)
	}
	data, err := s.eb.request(ctx, req)
	if err != nil {
		return nil, err
	}
	return map[string]any{"status": 200, "body": data}, nil
}

func paramsOrEmpty(params json.RawMessage) json.RawMessage {
	if len(params) == 0 || string(params) == "null" {
		return json.RawMessage(`{}`)
	}
	return params
}
