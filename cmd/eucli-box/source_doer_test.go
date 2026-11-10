package main

import (
	"context"
	"net/http"
	"testing"

	"eucli-box/pkg/installsource"
	"eucli-box/pkg/types"
)

// recordingNetwork 记录门控放行后的真实请求地址。
type recordingNetwork struct {
	calls []string
}

func (r *recordingNetwork) Do(_ context.Context, request types.HTTPRequest) (types.HTTPResponse, error) {
	r.calls = append(r.calls, request.URL)
	return types.HTTPResponse{StatusCode: http.StatusOK, Body: []byte("ok")}, nil
}

func TestReleaseFetchDoerRoutesBySource(t *testing.T) {
	network := &recordingNetwork{}
	state := installsource.NewState(installsource.Config{
		Source: installsource.OfficialSource,
		Sources: []installsource.Source{
			{Name: "社区商店", Kind: installsource.SourceKindRemote, IndexURL: "https://store.example.com/store/index.json"},
		},
	}, nil)
	doer := releaseFetchDoer{
		official: boxOfficialHTTPDoer{network: network},
		remote:   newRemoteStoreDoer(network, state),
	}

	officialRequest, _ := http.NewRequest(http.MethodGet, "https://github.com/noelle-silva/eucli-box-ai-tools/releases/download/context7/v0.1.0/tool.zip", nil)
	if _, err := doer.Do(officialRequest); err != nil {
		t.Fatalf("official request error = %v", err)
	}
	remoteRequest, _ := http.NewRequest(http.MethodGet, "https://store.example.com/store/files/tool.zip", nil)
	if _, err := doer.Do(remoteRequest); err != nil {
		t.Fatalf("remote request error = %v", err)
	}
	otherRequest, _ := http.NewRequest(http.MethodGet, "https://evil.example.com/tool.zip", nil)
	if _, err := doer.Do(otherRequest); err == nil {
		t.Fatal("unregistered host error = nil")
	}
	if len(network.calls) != 2 {
		t.Fatalf("network calls = %#v", network.calls)
	}
}

func TestRemoteStoreDoerRequiresHTTPS(t *testing.T) {
	network := &recordingNetwork{}
	state := installsource.NewState(installsource.Config{
		Source: installsource.OfficialSource,
		Sources: []installsource.Source{
			{Name: "社区商店", Kind: installsource.SourceKindRemote, IndexURL: "https://store.example.com/index.json"},
		},
	}, nil)
	doer := newRemoteStoreDoer(network, state)
	request, _ := http.NewRequest(http.MethodGet, "http://store.example.com/index.json", nil)
	if _, err := doer.Do(request); err == nil {
		t.Fatal("plain http error = nil")
	}
}
