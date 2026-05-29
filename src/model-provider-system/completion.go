package modelprovider

import (
	"context"

	"eucli-box/pkg/types"
)

func (s *system) Complete(ctx context.Context, request types.ModelRequest) (types.ModelResponse, error) {
	provider, _, err := s.ResolveModel(ctx, request.Coordinate)
	if err != nil {
		return types.ModelResponse{}, err
	}
	adapter, err := adapterFor(provider.Protocol)
	if err != nil {
		return types.ModelResponse{}, err
	}
	httpReq, err := adapter.BuildCompleteRequest(provider, request, int64(s.config.RequestTimeout))
	if err != nil {
		return types.ModelResponse{}, err
	}
	httpResp, err := s.network.Do(ctx, httpReq)
	if err != nil {
		return types.ModelResponse{}, providerNetworkFailed("failed to request provider completion", err)
	}
	return adapter.ParseCompleteResponse(httpResp)
}
