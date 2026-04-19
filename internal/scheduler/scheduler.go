package scheduler

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/vsangava/distractions-free/internal/config"
	"github.com/vsangava/distractions-free/internal/proxy"
)

var activeBlocks = make(map[string]bool)

// ScriptExecutor interface for testing AppleScript execution
type ScriptExecutor interface {
	ExecuteScript(script string) error
	LogScript(script string)
}

// MacOSScriptExecutor executes scripts on macOS
type MacOSScriptExecutor struct{}

func (e *MacOSScriptExecutor) ExecuteScript(script string) error {
	runAsMacUser(script)
	return nil // runAsMacUser doesn't return an error, so we assume success
}

func (e *MacOSScriptExecutor) LogScript(script string) {
	log.Printf("AppleScript generated:\n%s", script)
}

// TestScriptExecutor logs scripts instead of executing them
type TestScriptExecutor struct {
	executedScripts []string
}

func (e *TestScriptExecutor) ExecuteScript(script string) error {
	e.executedScripts = append(e.executedScripts, script)
	e.LogScript(script)
	return nil
}

func (e *TestScriptExecutor) LogScript(script string) {
	log.Printf("TEST MODE - AppleScript would execute:\n%s", script)
}

// Global executor - can be replaced for testing
var scriptExecutor ScriptExecutor = &MacOSScriptExecutor{}

// AppleScriptGenerator interface for testing script generation
type AppleScriptGenerator interface {
	GenerateWarningScript(domains []string) string
	GenerateCloseTabsScript(domains []string) string
}

// MacOSAppleScriptGenerator generates macOS AppleScripts
type MacOSAppleScriptGenerator struct{}

func (g *MacOSAppleScriptGenerator) GenerateWarningScript(domains []string) string {
	msg := fmt.Sprintf("Tabs for %s will close in 3 minutes.", strings.Join(domains, ", "))
	return fmt.Sprintf(`display notification "%s" with title "Distractions-Free" subtitle "Upcoming Block" sound name "Basso"`, msg)
}

func (g *MacOSAppleScriptGenerator) GenerateCloseTabsScript(domains []string) string {
	var quotedDomains []string
	for _, d := range domains {
		quotedDomains = append(quotedDomains, fmt.Sprintf(`"%s"`, strings.TrimPrefix(d, "www.")))
	}
	domainListStr := "{" + strings.Join(quotedDomains, ", ") + "}"

	return fmt.Sprintf(`
		set domainsToBlock to %s
		
		if application "Google Chrome" is running then
			tell application "Google Chrome"
				set tabsToClose to {}
				repeat with w in windows
					repeat with t in tabs of w
						set tabURL to URL of t
						repeat with d in domainsToBlock
							if tabURL contains d then
								set end of tabsToClose to t
								exit repeat
							end if
						end repeat
					end repeat
				end repeat
				repeat with t in tabsToClose
					close t
				end repeat
			end tell
		end if

		if application "Safari" is running then
			tell application "Safari"
				set tabsToClose to {}
				repeat with w in windows
					repeat with t in tabs of w
						set tabURL to URL of t
						repeat with d in domainsToBlock
							if tabURL contains d then
								set end of tabsToClose to t
								exit repeat
							end if
						end repeat
					end repeat
				end repeat
				repeat with t in tabsToClose
					close t
				end repeat
			end tell
		end if
	`, domainListStr)
}

// Global generator - can be replaced for testing
var scriptGenerator AppleScriptGenerator = &MacOSAppleScriptGenerator{}

// GetScriptGenerator returns the current script generator (for testing)
func GetScriptGenerator() AppleScriptGenerator {
	return scriptGenerator
}

// SetScriptGenerator sets the script generator (for testing)
func SetScriptGenerator(generator AppleScriptGenerator) {
	scriptGenerator = generator
}

// GetScriptExecutor returns the current script executor (for testing)
func GetScriptExecutor() ScriptExecutor {
	return scriptExecutor
}

// SetScriptExecutor sets the script executor (for testing)
func SetScriptExecutor(executor ScriptExecutor) {
	scriptExecutor = executor
}

// EvaluateRulesAtTime evaluates blocking rules at a specific time and returns blocked domains.
// This is the testable function that doesn't depend on time.Now().
func EvaluateRulesAtTime(t time.Time, cfg config.Config) map[string]bool {
	currentDay := t.Weekday().String()
	currentTime := t.Format("15:04")

	newBlocked := make(map[string]bool)

	// Evaluate times
	for _, rule := range cfg.Rules {
		if !rule.IsActive {
			continue
		}
		if slots, exists := rule.Schedules[currentDay]; exists {
			for _, slot := range slots {
				// Check active blocks
				if currentTime >= slot.Start && currentTime < slot.End {
					newBlocked[rule.Domain] = true
					break
				}
			}
		}
	}

	return newBlocked
}

// CheckWarningDomainsAtTime checks if any domains should trigger 3-minute warnings at a specific time.
// This is the testable function that doesn't depend on time.Now().
func CheckWarningDomainsAtTime(t time.Time, cfg config.Config) []string {
	currentDay := t.Weekday().String()
	futureTime := t.Add(3 * time.Minute).Format("15:04")

	var warningDomains []string

	// Check for 3-minute warnings
	for _, rule := range cfg.Rules {
		if !rule.IsActive {
			continue
		}
		if slots, exists := rule.Schedules[currentDay]; exists {
			for _, slot := range slots {
				if futureTime == slot.Start {
					warningDomains = append(warningDomains, rule.Domain)
				}
			}
		}
	}

	return warningDomains
}

func Start() {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		evaluateRules() // Run immediately
		for range ticker.C {
			evaluateRules()
		}
	}()
}

func evaluateRules() {
	cfg := config.GetConfig()
	now := time.Now()

	newBlocked := EvaluateRulesAtTime(now, cfg)
	warningDomains := CheckWarningDomainsAtTime(now, cfg)

	var newlyBlockedDomains []string
	requiresFlush := false

	// Check if state changed (domains added or removed)
	if len(newBlocked) != len(activeBlocks) || len(newlyBlockedDomains) > 0 {
		for domain := range newBlocked {
			if !activeBlocks[domain] {
				newlyBlockedDomains = append(newlyBlockedDomains, domain)
			}
		}
		if len(newlyBlockedDomains) > 0 {
			requiresFlush = true
		}
	}

	// Apply states
	activeBlocks = newBlocked
	proxy.UpdateBlockedDomains(newBlocked)

	if len(warningDomains) > 0 {
		runMacOSWarning(warningDomains)
	}

	if requiresFlush {
		flushDNS()
		if len(newlyBlockedDomains) > 0 {
			closeMacOSTabs(newlyBlockedDomains)
		}
	}
}

func runAsMacUser(scriptContent string) {
	if runtime.GOOS != "darwin" {
		return
	}

	scriptPath := "/tmp/df_script.scpt"
	os.WriteFile(scriptPath, []byte(scriptContent), 0644)
	exec.Command("osascript", scriptPath).Run()
}

func runMacOSWarning(domains []string) {
	script := scriptGenerator.GenerateWarningScript(domains)
	scriptExecutor.LogScript(script)
	scriptExecutor.ExecuteScript(script)
}

func closeMacOSTabs(domains []string) {
	script := scriptGenerator.GenerateCloseTabsScript(domains)
	scriptExecutor.LogScript(script)
	scriptExecutor.ExecuteScript(script)
}

func flushDNS() {
	if runtime.GOOS == "darwin" {
		exec.Command("dscacheutil", "-flushcache").Run()
		exec.Command("killall", "-HUP", "mDNSResponder").Run()
		log.Println("macOS DNS Cache Flushed.")
	} else if runtime.GOOS == "windows" {
		exec.Command("ipconfig", "/flushdns").Run()
	}
}
