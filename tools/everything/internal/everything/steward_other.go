//go:build !windows

package everything

import "context"

// productionStewardSystem returns the steward system of unsupported
// platforms: every external steward step reports errStewardUnsupported and
// full-disk search keeps its existing behavior.
func productionStewardSystem() stewardSystem {
	return stewardSystem{
		protectedBaseDir: func() (string, error) { return "", errStewardUnsupported },
		queryService: func(context.Context, string) (stewardServiceStatus, error) {
			return stewardServiceStatus{}, errStewardUnsupported
		},
		deleteService:  func(context.Context, string) error { return errStewardUnsupported },
		startService:   func(context.Context, string) error { return errStewardUnsupported },
		installService: func(context.Context, string, string) error { return errStewardUnsupported },
		elevateInstall: func(string) error { return errStewardUnsupported },
	}
}
