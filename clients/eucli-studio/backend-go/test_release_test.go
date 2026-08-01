package main

import (
	"context"

	"eucli-box/pkg/releasecheck"
	"eucli-box/pkg/types"
)

func testClientRelease() clientRelease {
	return clientRelease{
		Version: "0.1.9",
		EucliBoxCompatibility: types.EucliBoxCompatibility{
			MinimumVersion:          "0.1.0",
			MaximumVersionExclusive: "0.2.0",
		},
	}
}

func newBusinessReadyTestService(config *configStore, hub *eventHub) *service {
	svc, err := newService(config, testClientRelease(), hub, fakeClientReleaseChecker{})
	if err != nil {
		panic(err)
	}
	svc.setConnectionState(runtimeBootstrap{BusinessAvailable: true})
	return svc
}

type fakeClientReleaseChecker struct {
	snapshot types.ReleaseCheckSnapshot
}

func (f fakeClientReleaseChecker) CheckOnly(context.Context, []releasecheck.InstalledArtifact, string, []types.ReleaseArtifactIdentity) types.ReleaseCheckSnapshot {
	if f.snapshot.Status == "" {
		return releasecheck.PendingSnapshot()
	}
	return f.snapshot
}
