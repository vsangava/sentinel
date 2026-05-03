package pf

import (
	"os"
	"strings"
	"testing"
)

func TestGenerateAnchorContent_empty(t *testing.T) {
	got := GenerateAnchorContent(nil)
	if !strings.Contains(got, "no IPs") {
		t.Errorf("expected no-IPs comment, got %q", got)
	}
}

func TestGenerateAnchorContent_withIPs(t *testing.T) {
	ips := []string{"1.2.3.4", "5.6.7.8", "2001:db8::1"}
	got := GenerateAnchorContent(ips)

	for _, ip := range ips {
		if !strings.Contains(got, ip) {
			t.Errorf("missing IP %s in anchor content", ip)
		}
	}
	if !strings.Contains(got, "block drop out quick inet proto {tcp udp}") {
		t.Error("missing inet block rule")
	}
	if !strings.Contains(got, "block drop out quick inet6 proto {tcp udp}") {
		t.Error("missing inet6 block rule")
	}
	if strings.Contains(got, "table") {
		t.Error("anchor content must not use table declarations (unsupported on macOS)")
	}
}

func TestGenerateAnchorContentMixed_bothSections(t *testing.T) {
	blockIPs := []string{"1.2.3.4", "2001:db8::1"}
	dohIPs := []string{"1.1.1.1", "2606:4700:4700::1111"}
	got := GenerateAnchorContentMixed(blockIPs, dohIPs)

	// Section 1: all-port for blockIPs, both families.
	if !strings.Contains(got, "block drop out quick inet proto {tcp udp} from any to { 1.2.3.4 }") {
		t.Errorf("section 1 missing inet all-port rule:\n%s", got)
	}
	if !strings.Contains(got, "block drop out quick inet6 proto {tcp udp} from any to { 2001:db8::1 }") {
		t.Errorf("section 1 missing inet6 all-port rule:\n%s", got)
	}

	// Section 2: port-restricted for dohIPs.
	if !strings.Contains(got, "block drop out quick inet proto tcp from any to { 1.1.1.1 } port 443") {
		t.Errorf("section 2 missing inet DoH port 443 rule:\n%s", got)
	}
	if !strings.Contains(got, "block drop out quick inet proto {tcp udp} from any to { 1.1.1.1 } port 853") {
		t.Errorf("section 2 missing inet DoT port 853 rule:\n%s", got)
	}
	if !strings.Contains(got, "block drop out quick inet6 proto tcp from any to { 2606:4700:4700::1111 } port 443") {
		t.Errorf("section 2 missing inet6 DoH port 443 rule:\n%s", got)
	}
	if !strings.Contains(got, "block drop out quick inet6 proto {tcp udp} from any to { 2606:4700:4700::1111 } port 853") {
		t.Errorf("section 2 missing inet6 DoT port 853 rule:\n%s", got)
	}

	// Comments make the two sections visually distinct in the anchor file.
	if !strings.Contains(got, "section 1") || !strings.Contains(got, "section 2") {
		t.Error("expected section header comments in mixed content")
	}
}

func TestGenerateAnchorContentMixed_onlyDOH(t *testing.T) {
	got := GenerateAnchorContentMixed(nil, []string{"1.1.1.1"})
	if strings.Contains(got, "section 1") {
		t.Errorf("should not emit section 1 header when blockIPs is empty:\n%s", got)
	}
	if !strings.Contains(got, "port 443") || !strings.Contains(got, "port 853") {
		t.Errorf("section 2 must emit port-restricted rules:\n%s", got)
	}
	// Crucially: must NOT emit an unconditional all-port block on the DoH IP.
	// That would also drop UDP/53 to 1.1.1.1, breaking the daemon's own backup_dns.
	if strings.Contains(got, "to { 1.1.1.1 }\n") {
		t.Errorf("DoH IP must not appear in any unrestricted-port rule:\n%s", got)
	}
}

func TestGenerateAnchorContentMixed_onlyBlock(t *testing.T) {
	got := GenerateAnchorContentMixed([]string{"5.6.7.8"}, nil)
	if strings.Contains(got, "section 2") {
		t.Errorf("should not emit section 2 header when dohIPs is empty:\n%s", got)
	}
	if !strings.Contains(got, "block drop out quick inet proto {tcp udp} from any to { 5.6.7.8 }") {
		t.Errorf("section 1 missing all-port rule:\n%s", got)
	}
	if strings.Contains(got, "port 443") || strings.Contains(got, "port 853") {
		t.Errorf("no port-restricted rules expected when dohIPs is empty:\n%s", got)
	}
}

func TestGenerateAnchorContentMixed_bothEmpty(t *testing.T) {
	got := GenerateAnchorContentMixed(nil, nil)
	if !strings.Contains(got, "no IPs") {
		t.Errorf("expected no-IPs comment when both sets empty, got %q", got)
	}
	if strings.Contains(got, "block drop out") {
		t.Errorf("expected no block rules when both sets empty, got %q", got)
	}
}

func TestGeneratePreview_structure(t *testing.T) {
	p := GeneratePreview([]string{"google.com"}, "8.8.8.8:53")

	if len(p.Domains) != 1 || p.Domains[0] != "google.com" {
		t.Errorf("unexpected domains: %v", p.Domains)
	}
	if p.AnchorContent == "" {
		t.Error("anchor content should not be empty")
	}
	ips, ok := p.ResolvedIPs["google.com"]
	if !ok {
		t.Fatal("no resolved IPs for google.com")
	}
	if len(ips) == 0 {
		t.Skip("no IPs resolved — possible offline environment")
	}
}

func TestResolveDomainIPs_returnsIPs(t *testing.T) {
	ips := ResolveDomainIPs("google.com", "8.8.8.8:53")
	if len(ips) == 0 {
		t.Skip("no IPs resolved — possible offline/CI environment")
	}
	for _, ip := range ips {
		if ip == "" {
			t.Error("empty IP in result")
		}
	}
}

func TestResolveDomainIPs_deduplication(t *testing.T) {
	ips := ResolveDomainIPs("youtube.com", "8.8.8.8:53")
	seen := make(map[string]bool)
	for _, ip := range ips {
		if seen[ip] {
			t.Errorf("duplicate IP %s in result", ip)
		}
		seen[ip] = true
	}
}

func TestPFConfInjectAndStrip(t *testing.T) {
	original := "# pf.conf\nset skip on lo\n"
	tmp := t.TempDir() + "/pf.conf"

	if err := os.WriteFile(tmp, []byte(original), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Inject — replicate injectPFConf logic against tmp path.
	content, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), markerBeg) {
		t.Fatal("marker already present before inject")
	}

	injection := "\n" + markerBeg + "\n" + anchorLine + "\n" + loadLine + "\n" + markerEnd + "\n"
	injected := string(content) + injection
	if err := os.WriteFile(tmp, []byte(injected), 0644); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(tmp)
	if !strings.Contains(string(got), markerBeg) {
		t.Error("marker not found after inject")
	}
	if !strings.Contains(string(got), anchorLine) {
		t.Error("anchor line missing after inject")
	}

	// Strip — replicate stripPFConf logic.
	var outLines []string
	skip := false
	for _, line := range strings.Split(injected, "\n") {
		if line == markerBeg {
			skip = true
			continue
		}
		if line == markerEnd {
			skip = false
			continue
		}
		if !skip {
			outLines = append(outLines, line)
		}
	}
	stripped := strings.TrimRight(strings.Join(outLines, "\n"), "\n") + "\n"

	if strings.Contains(stripped, markerBeg) {
		t.Error("marker still present after strip")
	}
	if !strings.Contains(stripped, "set skip on lo") {
		t.Error("original content lost after strip")
	}
}
