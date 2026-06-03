package shellcommand

import (
	"fmt"
	"strings"
)

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
		env = append(env, "LANG=C.UTF-8", "LC_ALL=C.UTF-8")
	}
	return env
}

func powershellCommand(command string) string {
	prefix := "[Console]::InputEncoding = [Console]::OutputEncoding = [System.Text.UTF8Encoding]::new(); $OutputEncoding = [Console]::OutputEncoding; if ($PSVersionTable.PSVersion.Major -ge 7) { $PSStyle.OutputRendering = 'PlainText' }; "
	return prefix + command
}
