package modelprovider

import (
	"context"

	"eucli-box/pkg/types"
)

func (s *system) Complete(ctx context.Context, request types.ModelRequest) (types.ModelResponse, error) {
	request.Stream = false
	provider, resolved, err := s.ResolveModel(ctx, request.Coordinate)
	if err != nil {
		return types.ModelResponse{}, err
	}
	request.Coordinate.ProviderID = provider.ID
	request.Coordinate.ProviderName = provider.Name
	request.Coordinate.ModelID = resolved.ID
	if err := applyResolvedReasoning(&request, resolved); err != nil {
		return types.ModelResponse{}, err
	}
	provider, err = s.providerWithSelectedKey(provider)
	if err != nil {
		return types.ModelResponse{}, err
	}
	adapter, err := adapterFor(provider.Protocol)
	if err != nil {
		return types.ModelResponse{}, err
	}
	requestConfig, err := s.modelRequestConfig(ctx)
	if err != nil {
		return types.ModelResponse{}, err
	}
	httpReq, err := adapter.BuildCompleteRequest(provider, request, int64(timeoutFromMs(requestConfig.CompletionTimeoutMs)))
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
	return response, nil
}

func (s *system) CompleteStream(ctx context.Context, request types.ModelRequest, onEvent types.ModelStreamHandler) (types.ModelResponse, error) {
	request.Stream = true
	provider, resolved, err := s.ResolveModel(ctx, request.Coordinate)
	if err != nil {
		return types.ModelResponse{}, err
	}
	request.Coordinate.ProviderID = provider.ID
	request.Coordinate.ProviderName = provider.Name
	request.Coordinate.ModelID = resolved.ID
	if err := applyResolvedReasoning(&request, resolved); err != nil {
		return types.ModelResponse{}, err
	}
	provider, err = s.providerWithSelectedKey(provider)
	if err != nil {
		return types.ModelResponse{}, err
	}
	adapter, err := adapterFor(provider.Protocol)
	if err != nil {
		return types.ModelResponse{}, err
	}
	requestConfig, err := s.modelRequestConfig(ctx)
	if err != nil {
		return types.ModelResponse{}, err
	}
	httpReq, err := adapter.BuildCompleteRequest(provider, request, int64(timeoutFromMs(requestConfig.StreamIdleTimeoutMs)))
	if err != nil {
		return types.ModelResponse{}, err
	}
	parser := adapter.NewCompleteStreamParser(onEvent)
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
			return types.ModelResponse{}, streamParseErr
		}
		return types.ModelResponse{}, providerNetworkFailed("failed to stream provider completion", err)
	}
	response, parseErr := parser.Finish(httpResp)
	if parseErr != nil {
		return types.ModelResponse{}, parseErr
	}
	return response, nil
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
