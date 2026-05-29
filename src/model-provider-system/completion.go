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
	record := types.CallRecord{
		ID:         newCallID(),
		ProviderID: request.Coordinate.ProviderID,
		ModelID:    request.Coordinate.ModelID,
		CreatedAt:  time.Now().UTC(),
	}
	provider, _, err := s.ResolveModel(ctx, request.Coordinate)
	if err != nil {
		record.Success = false
		record.ErrorCode = errCode(err)
		_ = s.storage.SaveCallRecord(ctx, record)
		return types.ModelResponse{}, err
	}
	record.ProviderID = provider.ID
	adapter, err := adapterFor(provider.Protocol)
	if err != nil {
		record.Success = false
		record.ErrorCode = errCode(err)
		_ = s.storage.SaveCallRecord(ctx, record)
		return types.ModelResponse{}, err
	}
	httpReq, err := adapter.BuildCompleteRequest(provider, request, int64(s.config.RequestTimeout))
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

func newCallID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("call-%d-%x", time.Now().UTC().UnixNano(), b)
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
