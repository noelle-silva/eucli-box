package shellcommand

import (
	"fmt"
	"strings"
)

var utf8ProviderEnvironment = []string{
	"LANG=C.UTF-8",
	"LC_ALL=C.UTF-8",
	"PYTHONIOENCODING=utf-8",
}

func providerArgs(provider ProviderConfig, command string) ([]string, error) {
	switch strings.TrimSpace(provider.Kind) {
	case "git-bash":
		return []string{"--noprofile", "--norc", "-lc", command}, nil
	case "powershell-core":
		return []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", powershellCommand(command)}, nil
	case "nushell":
		return []string{"-c", command}, nil
	default:
		return nil, fmt.Errorf("provider %q kind %q is unsupported", provider.ID, provider.Kind)
	}
}

func providerEnv(provider ProviderConfig, base []string) []string {
	env := append([]string{}, base...)
	if strings.ToLower(strings.TrimSpace(provider.Encoding)) == "utf-8" {
		env = applyEnvironmentOverrides(env, utf8ProviderEnvironment)
	}
	return env
}

func applyEnvironmentOverrides(base []string, overrides []string) []string {
	overrideKeys := map[string]struct{}{}
	for _, override := range overrides {
		key, ok := environmentKey(override)
		if ok {
			overrideKeys[strings.ToUpper(key)] = struct{}{}
		}
	}
	env := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, ok := environmentKey(entry)
		if ok {
			if _, exists := overrideKeys[strings.ToUpper(key)]; exists {
				continue
			}
		}
		env = append(env, entry)
	}
	return append(env, overrides...)
}

func environmentKey(entry string) (string, bool) {
	key, _, ok := strings.Cut(entry, "=")
	return key, ok && key != ""
}

func powershellCommand(command string) string {
	prefix := "[Console]::InputEncoding = [Console]::OutputEncoding = [System.Text.UTF8Encoding]::new(); $OutputEncoding = [Console]::OutputEncoding; if ($PSVersionTable.PSVersion.Major -ge 7) { $PSStyle.OutputRendering = 'PlainText' }; "
	return prefix + command
}
