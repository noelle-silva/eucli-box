package modelprovider

import (
	"context"
	"time"

	"eucli-box/pkg/types"
)

// Complete 发起一次非流式模型请求；坐标指向 fallback 组时按成员顺序顺位兜底。
func (s *system) Complete(ctx context.Context, request types.ModelRequest) (types.ModelResponse, error) {
	request.Stream = false
	plan, err := s.planRequest(ctx, request.Coordinate)
	if err != nil {
		return types.ModelResponse{}, err
	}
	return s.runPlan(plan, func(candidate planCandidate) (types.ModelResponse, bool, error) {
		response, err := s.completeOnce(ctx, request, candidate)
		return response, false, err
	})
}

// CompleteStream 发起一次流式模型请求；一旦已经向调用方分发过内容就不再换人。
func (s *system) CompleteStream(ctx context.Context, request types.ModelRequest, onEvent types.ModelStreamHandler) (types.ModelResponse, error) {
	request.Stream = true
	plan, err := s.planRequest(ctx, request.Coordinate)
	if err != nil {
		return types.ModelResponse{}, err
	}
	return s.runPlan(plan, func(candidate planCandidate) (types.ModelResponse, bool, error) {
		return s.completeStreamOnce(ctx, request, candidate, onEvent)
	})
}

// runPlan 沿执行计划依次尝试候选：普通计划失败即止，兜底链顺位往下。
// 成员责任的失败记一次熔断记忆，成功则抹掉它的记录。
// attempt 回报本次尝试是否已经把内容交给调用方——交给过就不能再换人。
func (s *system) runPlan(plan requestPlan, attempt func(planCandidate) (types.ModelResponse, bool, error)) (types.ModelResponse, error) {
	failures := make([]failedMember, 0, len(plan.candidates))
	for index, candidate := range plan.candidates {
		response, dispatched, err := attempt(candidate)
		if err == nil {
			s.breaker.clear(candidate.key)
			return response, nil
		}
		if isMemberFailure(err) {
			s.breaker.trip(candidate.key)
		}
		failures = append(failures, failedMember{label: candidate.label(), err: err})
		if !plan.fallback() {
			return types.ModelResponse{}, err
		}
		if plan.shouldTryNext(index, err, dispatched) {
			continue
		}
		if plan.isLast(index) {
			return types.ModelResponse{}, aggregateMemberFailures(failures)
		}
		return types.ModelResponse{}, err
	}
	return types.ModelResponse{}, aggregateMemberFailures(failures)
}

// completeOnce 对单个候选发起一次非流式请求。
func (s *system) completeOnce(ctx context.Context, request types.ModelRequest, candidate planCandidate) (types.ModelResponse, error) {
	requestConfig, err := s.modelRequestConfig(ctx)
	if err != nil {
		return types.ModelResponse{}, err
	}
	httpReq, adapter, requestModel, err := s.buildCompleteRequest(request, candidate, timeoutFromMs(requestConfig.CompletionTimeoutMs))
	if err != nil {
		return types.ModelResponse{}, err
	}
	httpResp, err := s.network.Do(ctx, httpReq)
	if err != nil {
		return types.ModelResponse{}, providerNetworkFailed("failed to request provider completion", err)
	}
	response, parseErr := adapter.ParseCompleteResponse(httpResp)
	if parseErr != nil {
		return types.ModelResponse{}, parseErr
	}
	response.RequestModel = requestModel
	return response, nil
}

// completeStreamOnce 对单个候选发起一次流式请求，并回报是否已经分发过内容。
func (s *system) completeStreamOnce(ctx context.Context, request types.ModelRequest, candidate planCandidate, onEvent types.ModelStreamHandler) (types.ModelResponse, bool, error) {
	requestConfig, err := s.modelRequestConfig(ctx)
	if err != nil {
		return types.ModelResponse{}, false, err
	}
	httpReq, adapter, requestModel, err := s.buildCompleteRequest(request, candidate, timeoutFromMs(requestConfig.StreamIdleTimeoutMs))
	if err != nil {
		return types.ModelResponse{}, false, err
	}
	dispatched := false
	// 没有订阅方时保持 nil：没有人收到过内容，换人就不会产生重放。
	handler := onEvent
	if onEvent != nil {
		handler = func(event types.ModelStreamEvent) error {
			dispatched = true
			return onEvent(event)
		}
	}
	parser := adapter.NewCompleteStreamParser(handler)
	var streamParseErr error
	httpResp, err := s.network.DoStream(ctx, httpReq, func(chunk types.HTTPStreamChunk) error {
		if acceptErr := parser.Accept(chunk.Data); acceptErr != nil {
			streamParseErr = acceptErr
			return acceptErr
		}
		return nil
	})
	if err != nil {
		if streamParseErr != nil {
			return types.ModelResponse{}, dispatched, streamParseErr
		}
		return types.ModelResponse{}, dispatched, providerNetworkFailed("failed to stream provider completion", err)
	}
	response, parseErr := parser.Finish(httpResp)
	if parseErr != nil {
		return types.ModelResponse{}, dispatched, parseErr
	}
	response.RequestModel = requestModel
	return response, dispatched, nil
}

// buildCompleteRequest 按候选组装一次上游请求：坐标、推理配置、API Key 与协议适配器都在这里落定。
// requestModel 记录实际发出去的是哪一家供应商的哪个模型，供响应回填。
func (s *system) buildCompleteRequest(request types.ModelRequest, candidate planCandidate, timeout time.Duration) (types.HTTPRequest, protocolAdapter, *types.RequestModel, error) {
	provider := candidate.provider
	request.Coordinate.ProviderID = provider.ID
	request.Coordinate.ProviderName = provider.Name
	request.Coordinate.ModelID = candidate.model.ID
	// 实际发出去的就是这一家：模型组已在上一步解析成具体供应商与模型。
	requestModel := &types.RequestModel{ProviderID: provider.ID, ProviderName: provider.Name, ModelID: candidate.model.ID}
	if err := applyResolvedReasoning(&request, candidate.model); err != nil {
		return types.HTTPRequest{}, nil, nil, err
	}
	provider, err := s.providerWithSelectedKey(provider)
	if err != nil {
		return types.HTTPRequest{}, nil, nil, err
	}
	adapter, err := adapterFor(provider.Protocol)
	if err != nil {
		return types.HTTPRequest{}, nil, nil, err
	}
	httpReq, err := adapter.BuildCompleteRequest(provider, request, int64(timeout))
	if err != nil {
		return types.HTTPRequest{}, nil, nil, err
	}
	return httpReq, adapter, requestModel, nil
}

func applyResolvedReasoning(request *types.ModelRequest, resolved types.ModelInfo) error {
	effort := types.TrimReasoningEffort(request.ReasoningEffort)
	if effort != "" && !types.IsReasoningEffort(effort) {
		return providerInvalid("reasoningEffort is invalid", nil)
	}
	if !resolved.SupportsReasoning {
		request.ReasoningEffort = ""
		return nil
	}
	if effort == "" {
		effort = resolved.DefaultReasoningEffort
	}
	request.ReasoningEffort = types.NormalizeReasoningEffort(effort, types.DefaultReasoningEffort)
	return nil
}
