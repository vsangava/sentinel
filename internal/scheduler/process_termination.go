package scheduler

import (
	"path/filepath"
	"strings"
)

type blockedAppProcess struct {
	name    string
	domains []string
}

var blockedAppProcesses = []blockedAppProcess{
	{name: "roblox", domains: []string{"roblox.com", "rbxcdn.com"}},
	{name: "discord", domains: []string{"discord.com", "discordapp.com", "discordapp.net", "discord.gg", "discord.media"}},
}

func blockedProcessNames(blocked map[string]bool) []string {
	var names []string
	for _, app := range blockedAppProcesses {
		for domain, isBlocked := range blocked {
			if isBlocked && matchesAppDomain(domain, app.domains) {
				names = append(names, app.name)
				break
			}
		}
	}
	return names
}

func matchesAppDomain(domain string, markers []string) bool {
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	for _, marker := range markers {
		marker = strings.ToLower(marker)
		if domain == marker || strings.HasSuffix(domain, "."+marker) {
			return true
		}
	}
	return false
}

func processNameMatchesApp(processName, appName string) bool {
	name := strings.ToLower(filepath.Base(strings.TrimSpace(processName)))
	return strings.HasPrefix(name, strings.ToLower(appName))
}

func blockedProcessNamesForDomains(domains []string) []string {
	blocked := make(map[string]bool, len(domains))
	for _, domain := range domains {
		blocked[domain] = true
	}
	return blockedProcessNames(blocked)
}

func displayAppNames(apps []string) []string {
	names := make([]string, 0, len(apps))
	for _, app := range apps {
		switch app {
		case "roblox":
			names = append(names, "Roblox")
		case "discord":
			names = append(names, "Discord")
		}
	}
	return names
}

func runProcessWarning(domains []string, probe func([]string) []string, notify func([]string)) {
	apps := blockedProcessNamesForDomains(domains)
	if len(apps) == 0 {
		return
	}
	runningApps := probe(apps)
	if len(runningApps) > 0 {
		notify(runningApps)
	}
}

func runPerTickTerminateApps(blocked map[string]bool, terminate func([]string)) {
	apps := blockedProcessNames(blocked)
	if len(apps) > 0 {
		terminate(apps)
	}
}
