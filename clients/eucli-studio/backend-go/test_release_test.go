package main

import "eucli-box/pkg/types"

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
	svc := newService(config, testClientRelease(), hub)
	svc.setConnectionState(runtimeBootstrap{BusinessAvailable: true})
	return svc
}
