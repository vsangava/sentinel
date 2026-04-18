package proxy

import (
	"log"
	"strings"
	"sync"

	"github.com/miekg/dns"
	"github.com/vsangava/distractions-free/internal/config"
)

var (
	blockedDomains map[string]bool
	blockMu        sync.RWMutex
)

func UpdateBlockedDomains(newBlocked map[string]bool) {
	blockMu.Lock()
	defer blockMu.Unlock()
	blockedDomains = newBlocked
}

func handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = false

	if len(r.Question) == 0 {
		w.WriteMsg(m)
		return
	}

	q := r.Question[0]
	domain := strings.TrimSuffix(q.Name, ".")

	blockMu.RLock()
	isBlocked := blockedDomains[domain]
	blockMu.RUnlock()

	if isBlocked && q.Qtype == dns.TypeA {
		rr, _ := dns.NewRR(q.Name + " 60 IN A 0.0.0.0")
		m.Answer = append(m.Answer, rr)
		w.WriteMsg(m)
		return
	}

	cfg := config.GetConfig()
	c := new(dns.Client)

	in, _, err := c.Exchange(r, cfg.Settings.PrimaryDNS)
	if err != nil {
		in, _, err = c.Exchange(r, cfg.Settings.BackupDNS)
		if err != nil {
			dns.HandleFailed(w, r)
			return
		}
	}
	w.WriteMsg(in)
}

func StartDNSServer() {
	UpdateBlockedDomains(make(map[string]bool))
	dns.HandleFunc(".", handleDNSRequest)

	server := &dns.Server{Addr: "127.0.0.1:53", Net: "udp"}
	log.Printf("Starting local DNS proxy on 127.0.0.1:53...")
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Failed to start DNS server: %s", err.Error())
	}
}
