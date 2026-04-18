package testcli

import (
	"fmt"
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/vsangava/distractions-free/internal/config"
	"github.com/vsangava/distractions-free/internal/proxy"
	"github.com/vsangava/distractions-free/internal/scheduler"
)

const timeFormat = "2006-01-02 15:04"

// QueryBlocking tests whether a domain would be blocked at a specific time,
// and returns the DNS response (either 0.0.0.0 or upstream result).
func QueryBlocking(timeStr, domain string) error {
	// Parse the time
	testTime, err := time.Parse(timeFormat, timeStr)
	if err != nil {
		return fmt.Errorf("invalid time format. Use: %s (example: 2024-04-01 10:30)", timeFormat)
	}

	// Normalize domain (remove trailing dot if present)
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}

	// Load config
	if err := config.LoadConfig(); err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	cfg := config.GetConfig()

	// Evaluate blocking rules at this time
	blockedDomains := scheduler.EvaluateRulesAtTime(testTime, cfg)

	// Create a DNS query
	dnsQuery := new(dns.Msg)
	dnsQuery.SetQuestion(domain+".", dns.TypeA)

	// Get the DNS response
	response, err := proxy.GetDNSResponse(dnsQuery, blockedDomains, cfg.Settings.PrimaryDNS, cfg.Settings.BackupDNS)
	if err != nil {
		return fmt.Errorf("DNS query failed: %v", err)
	}

	// Format and print results
	isBlocked := blockedDomains[domain]

	separator := strings.Repeat("=", 60)
	dashLine := strings.Repeat("-", 60)

	fmt.Println(separator)
	fmt.Printf("Test Query Result\n")
	fmt.Println(separator)
	fmt.Printf("Time:          %s (%s)\n", testTime.Format(timeFormat), testTime.Weekday())
	fmt.Printf("Domain:        %s\n", domain)
	fmt.Println(dashLine)

	if isBlocked {
		fmt.Printf("Status:        🚫 BLOCKED\n")
		if len(response.Answer) > 0 {
			if a, ok := response.Answer[0].(*dns.A); ok {
				fmt.Printf("Response:      %s (blocking response)\n", a.A.String())
			}
		}
	} else {
		fmt.Printf("Status:        ✓ ALLOWED (forwarded to upstream DNS)\n")
		if len(response.Answer) > 0 {
			fmt.Printf("Response:      %v\n", response.Answer[0])
		} else {
			fmt.Printf("Response:      No DNS answer (domain may not exist or DNS error)\n")
		}
	}

	// Show blocking rules that apply
	fmt.Println(dashLine)
	fmt.Println("Applicable Rules:")

	foundRules := false
	for _, rule := range cfg.Rules {
		if !rule.IsActive {
			continue
		}

		if rule.Domain != domain {
			continue
		}

		foundRules = true
		fmt.Printf("  Domain: %s\n", rule.Domain)

		if slots, exists := rule.Schedules[testTime.Weekday().String()]; exists {
			for _, slot := range slots {
				currentTime := testTime.Format("15:04")
				if currentTime >= slot.Start && currentTime < slot.End {
					fmt.Printf("    ✓ Blocked on %s from %s to %s (ACTIVE)\n", testTime.Weekday(), slot.Start, slot.End)
				} else {
					fmt.Printf("    ○ Blocked on %s from %s to %s (not active now)\n", testTime.Weekday(), slot.Start, slot.End)
				}
			}
		} else {
			fmt.Printf("    ○ No schedule for %s\n", testTime.Weekday())
		}
	}

	if !foundRules {
		fmt.Println("  (No active rules for this domain)")
	}

	// Show warning info
	fmt.Println(dashLine)
	warnings := scheduler.CheckWarningDomainsAtTime(testTime, cfg)
	if contains(warnings, domain) {
		fmt.Printf("⚠️  Warning will trigger 3 minutes before block!\n")
	}

	fmt.Println(separator)

	return nil
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}
