package main

import (
	"fmt"
	"os"

	"eucli-box/pkg/systemplugin"
	"eucli-box/pkg/systemplugin/pluginrun"
)

const (
	weatherDetailInterfaceID = "weather-detail"
	weatherBriefInterfaceID  = "weather-brief"
)

func main() {
	if err := pluginrun.Serve(pluginrun.Options{
		PluginID: "weather-plugin",
		Capabilities: []systemplugin.Capability{{
			Type:       systemplugin.CapabilityPlaceholderValues,
			Interfaces: []string{weatherDetailInterfaceID, weatherBriefInterfaceID},
		}},
		Placeholder: newWeatherProvider(),
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
