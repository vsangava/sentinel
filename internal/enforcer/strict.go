package enforcer

import (
	"log"

	"github.com/vsangava/sentinel/internal/config"
	"github.com/vsangava/sentinel/internal/pf"
)

// dohGroupName names the special group whose domains are blocked at the DNS layer
// only and explicitly excluded from the pf IP-resolution path. The point is to
// avoid IP-blocking common DoH endpoints like 1.1.1.1 / 8.8.8.8 — those same IPs
// are routinely used as plain-DNS upstreams (including this daemon's backup_dns),
// and an unconditional pf rule on them would break the daemon's own resolver.
const dohGroupName = "_doh"

// StrictEnforcer composes DNS-proxy blocking with pf firewall blocking.
// Domains in the _doh group are blocked at the DNS layer only.
type StrictEnforcer struct {
	dns *DNSEnforcer
	cfg config.Config
}

func NewStrictEnforcer(cfg config.Config) *StrictEnforcer {
	return &StrictEnforcer{dns: NewDNSEnforcer(cfg), cfg: cfg}
}

func (e *StrictEnforcer) Setup() error {
	if err := e.dns.Setup(); err != nil {
		return err
	}
	if err := pf.InstallAnchor(); err != nil {
		log.Printf("strict: pf anchor setup failed, continuing with DNS-only: %v", err)
	}
	return nil
}

func (e *StrictEnforcer) Teardown() error {
	e.dns.Teardown()
	pf.RemoveAnchor()
	return nil
}

func (e *StrictEnforcer) Activate(domains []string) error {
	if err := e.dns.Activate(domains); err != nil {
		return err
	}
	e.reloadPF()
	return nil
}

func (e *StrictEnforcer) Deactivate(domains []string) error {
	if err := e.dns.Deactivate(domains); err != nil {
		return err
	}
	e.reloadPF()
	return nil
}

func (e *StrictEnforcer) DeactivateAll() error {
	e.dns.DeactivateAll()
	pf.DeactivateBlock()
	return nil
}

// Refresh re-resolves all currently blocked domains and reloads the pf anchor.
// Called every scheduler tick so CDN IP rotations are picked up without waiting
// for a domain set change to trigger Activate.
func (e *StrictEnforcer) Refresh() {
	e.reloadPF()
}

// reloadPF rewrites the pf anchor from the current DNS-blocked set, excluding
// domains in the _doh group (DNS-only). pf.ActivateBlock is an atomic anchor
// overwrite, so no DeactivateBlock call is needed before it — and adding one
// would briefly leave pf with no rules between the two reloads.
//
// Resolves a fresh config every call so a toggle of _doh.is_active or a domain
// addition takes effect on the next tick without restarting the enforcer.
func (e *StrictEnforcer) reloadPF() {
	cfg := config.GetConfig()
	skip := make(map[string]bool)
	for _, d := range cfg.ResolveGroup(dohGroupName) {
		skip[d] = true
	}

	e.dns.mu.Lock()
	domains := make([]string, 0, len(e.dns.blocked))
	for d := range e.dns.blocked {
		if skip[d] {
			continue
		}
		domains = append(domains, d)
	}
	e.dns.mu.Unlock()

	if len(domains) == 0 {
		pf.DeactivateBlock()
		return
	}
	pf.ActivateBlock(domains, cfg.Settings.PrimaryDNS, cfg.Settings.BackupDNS)
}
