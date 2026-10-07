package main

import (
	"fmt"
	"os"

	"eucli-box/system-plugins/weather-plugin/internal/pluginrun"
	"eucli-box/system-plugins/weather-plugin/internal/systemplugin"
)

const (
	weatherDetailInterfaceID = "weather-detail"
	weatherBriefInterfaceID  = "weather-brief"
)

func main() {
	if isDataMigrationMode(os.Args[1:]) {
		os.Exit(runDataMigration(os.Args[1:], os.Stdout))
	}
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
