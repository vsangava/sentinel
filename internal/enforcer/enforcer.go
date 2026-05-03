// Package enforcer defines the Enforcer interface and the factory that selects
// the right backend based on the configured enforcement_mode.
//
// Three modes are supported:
//   - "hosts"  (default) — edits /etc/hosts; no port binding required
//   - "dns"              — DNS proxy on 127.0.0.1:53; preserves legacy behaviour
//   - "strict"           — DNS proxy + pf firewall (pf integration is a stub for now)
package enforcer

import (
	"os/exec"
	"runtime"

	"github.com/vsangava/sentinel/internal/config"
	"github.com/vsangava/sentinel/internal/pf"
)

// Enforcer is implemented by every blocking backend.
// Setup/Teardown are called once at service start/stop.
// Activate/Deactivate are called with domain-level diffs each scheduler tick.
// Refresh is called every tick when blocks are active; strict mode uses it to
// re-resolve CDN IPs that rotate between Activate calls.
type Enforcer interface {
	Setup() error
	Teardown() error
	Activate(domains []string) error
	Deactivate(domains []string) error
	DeactivateAll() error
	Refresh()
}

// New returns the Enforcer matching the config's enforcement_mode.
//
// When the mode is hosts/dns, this also calls pf.RemoveAnchorIfPresent to clear any
// stale pf anchor left behind by a previous strict-mode run that exited without a
// graceful Stop (crash, SIGKILL, OOM). The graceful path already cleans pf via
// StrictEnforcer.Teardown; this is the self-heal for the ungraceful path so a
// downgrade from strict to hosts/dns can never silently inherit lingering firewall
// rules.
func New(cfg config.Config) Enforcer {
	mode := cfg.Settings.GetEnforcementMode()
	if mode != "strict" {
		pf.RemoveAnchorIfPresent()
	}
	switch mode {
	case "strict":
		return NewStrictEnforcer(cfg)
	case "dns":
		return NewDNSEnforcer(cfg)
	default: // "hosts" and any unrecognised value
		return NewHostsEnforcer(cfg)
	}
}

// flushDNSCache flushes the OS resolver cache. Called by enforcer backends after
// modifying blocking state so changes take effect without waiting for TTL expiry.
func flushDNSCache() {
	if runtime.GOOS == "darwin" {
		exec.Command("dscacheutil", "-flushcache").Run()
		exec.Command("killall", "-HUP", "mDNSResponder").Run()
	} else if runtime.GOOS == "windows" {
		exec.Command("ipconfig", "/flushdns").Run()
	}
}
