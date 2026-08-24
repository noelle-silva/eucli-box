package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

func TestLoadClientReleaseValidatesCompleteMetadata(t *testing.T) {
	info, err := loadClientRelease(`{"version":"0.1.9","eucliBoxCompatibility":{"minimumVersion":"0.1.0","maximumVersionExclusive":"0.2.0"}}`)
	if err != nil {
		t.Fatalf("loadClientRelease() error = %v", err)
	}
	if info.Version != "0.1.9" || info.EucliBoxCompatibility.MinimumVersion != "0.1.0" || info.EucliBoxCompatibility.MaximumVersionExclusive != "0.2.0" {
		t.Fatalf("release = %#v", info)
	}

	invalid := []string{
		`{"version":"0.1","eucliBoxCompatibility":{"minimumVersion":"0.1.0","maximumVersionExclusive":"0.2.0"}}`,
		`{"version":"0.1.9","eucliBoxCompatibility":{"minimumVersion":"0.2.0","maximumVersionExclusive":"0.1.0"}}`,
		`{"version":"0.1.9","eucliBoxCompatibility":{"minimumVersion":"0.1.0","maximumVersionExclusive":"0.2.0"},"unknown":true}`,
	}
	for _, source := range invalid {
		if _, err := loadClientRelease(source); err == nil {
			t.Fatalf("loadClientRelease(%s) should fail", source)
		}
	}
}

func TestEBClientAppliesReleaseHeaders(t *testing.T) {
	release := testClientRelease()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Eucli-Studio-Version"); got != release.Version {
			t.Errorf("version header = %q", got)
		}
		if got := r.Header.Get("X-Eucli-Studio-Minimum-Box-Version"); got != release.EucliBoxCompatibility.MinimumVersion {
			t.Errorf("minimum header = %q", got)
		}
		if got := r.Header.Get("X-Eucli-Studio-Maximum-Box-Version"); got != release.EucliBoxCompatibility.MaximumVersionExclusive {
			t.Errorf("maximum header = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"ok": true}})
	}))
	defer server.Close()

	store := configuredTestStore(t, server.URL)
	if _, err := newEBClient(store, release).request(context.Background(), ebRequest{Method: http.MethodGet, Path: "/test"}); err != nil {
		t.Fatalf("request() error = %v", err)
	}
}

func TestBootstrapRequiresCompatibleEucliBox(t *testing.T) {
	tests := []struct {
		name      string
		response  map[string]any
		available bool
	}{
		{
			name: "compatible",
			response: map[string]any{
				"version": "0.1.0",
				"clientCompatibility": map[string]any{
					"compatible":             true,
					"currentEucliBoxVersion": "0.1.0",
					"requiredEucliBoxCompatibility": map[string]any{
						"minimumVersion": "0.1.0", "maximumVersionExclusive": "0.2.0",
					},
				},
			},
			available: true,
		},
		{
			name: "incompatible",
			response: map[string]any{
				"version": "0.2.0",
				"clientCompatibility": map[string]any{
					"compatible":             false,
					"reason":                 "当前 eucli-box 版本不在所需范围",
					"currentEucliBoxVersion": "0.2.0",
					"requiredEucliBoxCompatibility": map[string]any{
						"minimumVersion": "0.1.0", "maximumVersionExclusive": "0.2.0",
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"data": test.response})
			}))
			defer server.Close()
			svc, err := newService(configuredTestStore(t, server.URL), testClientRelease(), nil)
			if err != nil {
				t.Fatalf("newService() error = %v", err)
			}
			info, err := svc.bootstrapConnected(context.Background(), runtimeBootstrap{})
			if err != nil {
				t.Fatalf("bootstrap() error = %v", err)
			}
			if info.BusinessAvailable != test.available {
				t.Fatalf("businessAvailable = %v, issue = %q", info.BusinessAvailable, info.EucliBoxIssue)
			}
			if !test.available && info.EucliBoxIssue == "" {
				t.Fatal("incompatible bootstrap should preserve the reason")
			}
		})
	}
}

func TestBootstrapManualConfigChain(t *testing.T) {
	store, err := newConfigStore(t.TempDir())
	if err != nil {
		t.Fatalf("newConfigStore() error = %v", err)
	}
	svc, err := newService(store, testClientRelease(), nil)
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}
	info, err := svc.bootstrap(context.Background())
	if err != nil {
		t.Fatalf("bootstrap() error = %v", err)
	}
	if info.EucliBoxConfigured || info.EucliBoxReachable {
		t.Fatalf("unconfigured bootstrap = %#v", info)
	}
	if info.EucliBoxIssue == "" {
		t.Fatal("unconfigured bootstrap should carry an issue")
	}
}

func TestBusinessMethodsRequireSuccessfulBootstrap(t *testing.T) {
	store, err := newConfigStore(t.TempDir())
	if err != nil {
		t.Fatalf("newConfigStore() error = %v", err)
	}
	svc, err := newService(store, testClientRelease(), nil)
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}
	_, err = svc.dispatch(context.Background(), "aiChat.storageGet", json.RawMessage(`{"key":"runtime/test"}`))
	var coded codedError
	if !errors.As(err, &coded) || coded.Code() != "EUCLI_BOX_CONNECTION_REQUIRED" {
		t.Fatalf("error = %#v", err)
	}

	svc.setConnectionState(runtimeBootstrap{BusinessAvailable: false, EucliBoxIssue: "版本不适用"})
	_, err = svc.dispatch(context.Background(), "aiChat.storageGet", json.RawMessage(`{"key":"runtime/test"}`))
	if !errors.As(err, &coded) || coded.Code() != "EUCLI_BOX_INCOMPATIBLE" {
		t.Fatalf("error = %#v", err)
	}
}

func TestReleaseCheckRefreshKeepsPreviousResultsOnTotalFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()
	svc, err := newService(configuredTestStore(t, server.URL), testClientRelease(), nil)
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}
	svc.setConnectionState(runtimeBootstrap{EucliBoxReachable: true})
	svc.storeReleaseCheckSnapshot(types.ReleaseCheckSnapshot{
		Status: types.ReleaseCheckStatusCompleted,
		Results: []types.ReleaseCheckResult{{
			Artifact:      types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"},
			Status:        types.ReleaseCheckStatusCompleted,
			LatestVersion: "0.1.0",
		}},
	})

	snapshot, err := svc.refreshReleaseChecks(context.Background(), "")
	if err != nil {
		t.Fatalf("refresh error = %v", err)
	}
	if snapshot.Status != types.ReleaseCheckStatusFailed || len(snapshot.Results) != 1 || snapshot.Results[0].LatestVersion != "0.1.0" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestReleaseCheckRefreshUsesConnectedBusinessBackend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/release-checks/refresh" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": types.ReleaseCheckSnapshot{
			Status: types.ReleaseCheckStatusCompleted,
			Results: []types.ReleaseCheckResult{{
				Artifact:      types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "time-plugin"},
				LatestVersion: "0.1.0",
				Status:        types.ReleaseCheckStatusCompleted,
			}},
		}})
	}))
	defer server.Close()

	svc, err := newService(configuredTestStore(t, server.URL), testClientRelease(), nil)
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}
	svc.setConnectionState(runtimeBootstrap{EucliBoxReachable: true})

	snapshot, err := svc.refreshReleaseChecks(context.Background(), "")
	if err != nil {
		t.Fatalf("refresh error = %v", err)
	}
	if snapshot.Status != types.ReleaseCheckStatusCompleted || len(snapshot.Results) != 1 || snapshot.Results[0].Artifact.ID != "time-plugin" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestReleaseCheckRefreshWithoutConnectionFails(t *testing.T) {
	store, err := newConfigStore(t.TempDir())
	if err != nil {
		t.Fatalf("newConfigStore() error = %v", err)
	}
	svc, err := newService(store, testClientRelease(), nil)
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}
	snapshot, err := svc.refreshReleaseChecks(context.Background(), "")
	if err != nil {
		t.Fatalf("refresh error = %v", err)
	}
	if snapshot.Status != types.ReleaseCheckStatusFailed {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestReleaseCheckRefreshDoesNotRunConcurrently(t *testing.T) {
	started := make(chan struct{})
	firstDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-started:
		default:
			close(started)
		}
		<-firstDone
		_ = json.NewEncoder(w).Encode(map[string]any{"data": types.ReleaseCheckSnapshot{
			Status:  types.ReleaseCheckStatusCompleted,
			Results: []types.ReleaseCheckResult{},
		}})
	}))
	defer server.Close()

	svc, err := newService(configuredTestStore(t, server.URL), testClientRelease(), nil)
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}
	svc.setConnectionState(runtimeBootstrap{EucliBoxReachable: true})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = svc.refreshReleaseChecks(context.Background(), "")
	}()
	<-started

	second := make(chan types.ReleaseCheckSnapshot, 1)
	go func() {
		snapshot, _ := svc.refreshReleaseChecks(context.Background(), "")
		second <- snapshot
	}()
	select {
	case snapshot := <-second:
		if snapshot.Status != types.ReleaseCheckStatusChecking {
			t.Fatalf("second snapshot status = %q", snapshot.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second refresh did not return while first is running")
	}
	close(firstDone)
	wg.Wait()
}

func configuredTestStore(t *testing.T, url string) *configStore {
	t.Helper()
	store, err := newConfigStore(t.TempDir())
	if err != nil {
		t.Fatalf("newConfigStore() error = %v", err)
	}
	if _, err := store.save(clientConfig{EucliBoxURL: url}); err != nil {
		t.Fatalf("save config error = %v", err)
	}
	return store
}
