package modelprovider

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	apperrors "eucli-box/pkg/errors"
	"eucli-box/pkg/types"
)

func (s *system) Complete(ctx context.Context, request types.ModelRequest) (types.ModelResponse, error) {
	request.Stream = false
	record := types.CallRecord{
		ID:         newCallID(),
		ProviderID: request.Coordinate.ProviderID,
		ModelID:    request.Coordinate.ModelID,
		CreatedAt:  time.Now().UTC(),
	}
	provider, resolved, err := s.ResolveModel(ctx, request.Coordinate)
	if err != nil {
		record.Success = false
		record.ErrorCode = errCode(err)
		_ = s.storage.SaveCallRecord(ctx, record)
		return types.ModelResponse{}, err
	}
	request.Coordinate.ProviderID = provider.ID
	request.Coordinate.ProviderName = provider.Name
	request.Coordinate.ModelID = resolved.ID
	if err := applyResolvedReasoning(&request, resolved); err != nil {
		record.Success = false
		record.ErrorCode = errCode(err)
		_ = s.storage.SaveCallRecord(ctx, record)
		return types.ModelResponse{}, err
	}
	record.ProviderID = provider.ID
	record.ModelID = resolved.ID
	provider, err = s.providerWithSelectedKey(provider)
	if err != nil {
		record.Success = false
		record.ErrorCode = errCode(err)
		_ = s.storage.SaveCallRecord(ctx, record)
		return types.ModelResponse{}, err
	}
	adapter, err := adapterFor(provider.Protocol)
	if err != nil {
		record.Success = false
		record.ErrorCode = errCode(err)
		_ = s.storage.SaveCallRecord(ctx, record)
		return types.ModelResponse{}, err
	}
	requestConfig, err := s.modelRequestConfig(ctx)
	if err != nil {
		record.Success = false
		record.ErrorCode = errCode(err)
		_ = s.storage.SaveCallRecord(ctx, record)
		return types.ModelResponse{}, err
	}
	httpReq, err := adapter.BuildCompleteRequest(provider, request, int64(timeoutFromMs(requestConfig.CompletionTimeoutMs)))
	if err != nil {
		record.Success = false
		record.ErrorCode = errCode(err)
		_ = s.storage.SaveCallRecord(ctx, record)
		return types.ModelResponse{}, err
	}
	httpResp, err := s.network.Do(ctx, httpReq)
	if err != nil {
		record.Success = false
		record.ErrorCode = errCode(err)
		_ = s.storage.SaveCallRecord(ctx, record)
		return types.ModelResponse{}, providerNetworkFailed("failed to request provider completion", err)
	}
	response, parseErr := adapter.ParseCompleteResponse(httpResp)
	if parseErr != nil {
		record.Success = false
		record.ErrorCode = errCode(parseErr)
		_ = s.storage.SaveCallRecord(ctx, record)
		return types.ModelResponse{}, parseErr
	}
	record.Success = true
	_ = s.storage.SaveCallRecord(ctx, record)
	return response, nil
}

func (s *system) CompleteStream(ctx context.Context, request types.ModelRequest, onEvent types.ModelStreamHandler) (types.ModelResponse, error) {
	request.Stream = true
	record := types.CallRecord{ID: newCallID(), ProviderID: request.Coordinate.ProviderID, ModelID: request.Coordinate.ModelID, CreatedAt: time.Now().UTC()}
	provider, resolved, err := s.ResolveModel(ctx, request.Coordinate)
	if err != nil {
		record.Success = false
		record.ErrorCode = errCode(err)
		_ = s.storage.SaveCallRecord(ctx, record)
		return types.ModelResponse{}, err
	}
	request.Coordinate.ProviderID = provider.ID
	request.Coordinate.ProviderName = provider.Name
	request.Coordinate.ModelID = resolved.ID
	if err := applyResolvedReasoning(&request, resolved); err != nil {
		record.Success = false
		record.ErrorCode = errCode(err)
		_ = s.storage.SaveCallRecord(ctx, record)
		return types.ModelResponse{}, err
	}
	record.ProviderID = provider.ID
	record.ModelID = resolved.ID
	provider, err = s.providerWithSelectedKey(provider)
	if err != nil {
		record.Success = false
		record.ErrorCode = errCode(err)
		_ = s.storage.SaveCallRecord(ctx, record)
		return types.ModelResponse{}, err
	}
	adapter, err := adapterFor(provider.Protocol)
	if err != nil {
		record.Success = false
		record.ErrorCode = errCode(err)
		_ = s.storage.SaveCallRecord(ctx, record)
		return types.ModelResponse{}, err
	}
	requestConfig, err := s.modelRequestConfig(ctx)
	if err != nil {
		record.Success = false
		record.ErrorCode = errCode(err)
		_ = s.storage.SaveCallRecord(ctx, record)
		return types.ModelResponse{}, err
	}
	httpReq, err := adapter.BuildCompleteRequest(provider, request, int64(timeoutFromMs(requestConfig.StreamIdleTimeoutMs)))
	if err != nil {
		record.Success = false
		record.ErrorCode = errCode(err)
		_ = s.storage.SaveCallRecord(ctx, record)
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
		record.Success = false
		record.ErrorCode = errCode(err)
		_ = s.storage.SaveCallRecord(ctx, record)
		if streamParseErr != nil {
			return types.ModelResponse{}, streamParseErr
		}
		return types.ModelResponse{}, providerNetworkFailed("failed to stream provider completion", err)
	}
	response, parseErr := parser.Finish(httpResp)
	if parseErr != nil {
		record.Success = false
		record.ErrorCode = errCode(parseErr)
		_ = s.storage.SaveCallRecord(ctx, record)
		return types.ModelResponse{}, parseErr
	}
	record.Success = true
	_ = s.storage.SaveCallRecord(ctx, record)
	return response, nil
}

func newCallID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("call-%d-%x", time.Now().UTC().UnixNano(), b)
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

func errCode(err error) string {
	if err == nil {
		return ""
	}
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return err.Error()
}
