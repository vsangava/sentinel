package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/vsangava/sentinel/internal/config"
)

// withTempConfigDir points config.ConfigDirOverride at a freshly-allocated
// temp dir for the duration of the test, restoring the prior value on cleanup.
// All usage-log paths key off ConfigDir(), so this is the only isolation
// hook the rotation tests need.
func withTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := config.ConfigDirOverride
	config.ConfigDirOverride = dir
	t.Cleanup(func() { config.ConfigDirOverride = prev })
	return dir
}

func TestUsageFilePathPerDate(t *testing.T) {
	dir := withTempConfigDir(t)

	when := time.Date(2026, 5, 7, 14, 32, 0, 0, time.Local)
	got := usageFilePathForDate(when)
	want := filepath.Join(dir, "usage-2026-05-07.jsonl")
	if got != want {
		t.Fatalf("usageFilePathForDate: got %q, want %q", got, want)
	}

	// Round-trip through parseUsageFileDate.
	parsed, ok := parseUsageFileDate(filepath.Base(got))
	if !ok {
		t.Fatalf("parseUsageFileDate rejected its own output %q", filepath.Base(got))
	}
	if !parsed.Equal(time.Date(2026, 5, 7, 0, 0, 0, 0, time.Local)) {
		t.Fatalf("parsed date: got %v, want 2026-05-07", parsed)
	}
}

func TestParseUsageFileDate_RejectsUnrelatedNames(t *testing.T) {
	bad := []string{
		"usage.jsonl",            // legacy single file
		"usage-2026-13-40.jsonl", // invalid date
		"usage-2026-05-07.txt",   // wrong extension
		"events.jsonl",
		"config.json",
		"random.jsonl",
	}
	for _, name := range bad {
		if _, ok := parseUsageFileDate(name); ok {
			t.Errorf("parseUsageFileDate accepted unrelated name %q", name)
		}
	}
}

func TestAppendThenReadAcrossDayBoundary(t *testing.T) {
	withTempConfigDir(t)

	day1 := time.Date(2026, 5, 6, 0, 0, 0, 0, time.Local)
	day2 := day1.AddDate(0, 0, 1)
	events := []UsageEvent{
		{TS: day1.Add(2 * time.Hour), Domain: "youtube.com", Group: "social", Kind: KindForeground},
		{TS: day1.Add(23*time.Hour + 30*time.Minute), Domain: "youtube.com", Group: "social", Kind: KindDNS},
		{TS: day2.Add(1 * time.Hour), Domain: "reddit.com", Group: "social", Kind: KindDNS},
		{TS: day2.Add(8 * time.Hour), Domain: "reddit.com", Group: "social", Kind: KindForeground},
	}
	for _, e := range events {
		if err := AppendUsageEvent(e); err != nil {
			t.Fatalf("AppendUsageEvent: %v", err)
		}
	}

	// Two distinct files should exist.
	files, err := listUsageFiles()
	if err != nil {
		t.Fatalf("listUsageFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("listUsageFiles: got %d files, want 2 — files=%v", len(files), files)
	}
	if !strings.Contains(filepath.Base(files[0]), "2026-05-06") || !strings.Contains(filepath.Base(files[1]), "2026-05-07") {
		t.Fatalf("files not sorted oldest-first: %v", files)
	}

	// Read with `since` mid-day-1 — must include the day-1 tail and all of day-2.
	since := day1.Add(20 * time.Hour)
	got, err := ReadUsageEventsSince(since)
	if err != nil {
		t.Fatalf("ReadUsageEventsSince: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ReadUsageEventsSince: got %d events, want 3 — %+v", len(got), got)
	}

	// Sorted by file order — the day-1 23:30 event must come before the day-2 events.
	if !got[0].TS.Before(got[1].TS) || !got[1].TS.Before(got[2].TS) {
		t.Errorf("ReadUsageEventsSince did not preserve chronological order across files: %+v", got)
	}
}

func TestPruneRemovesOldFiles(t *testing.T) {
	dir := withTempConfigDir(t)

	now := time.Now()
	// Write files dated -45d, -30d, -1d, today. After pruning at 30d, only -45d
	// should be gone — -30d sits exactly on the cutoff and is kept.
	stamps := map[string]time.Duration{
		"old":    -45 * 24 * time.Hour,
		"edge":   -30 * 24 * time.Hour,
		"recent": -1 * 24 * time.Hour,
		"today":  0,
	}
	paths := make(map[string]string)
	for label, age := range stamps {
		when := now.Add(age)
		path := filepath.Join(dir, "usage-"+when.Format(usageDateLayout)+".jsonl")
		if err := os.WriteFile(path, []byte("{\"ts\":\"2026-01-01T00:00:00Z\",\"domain\":\"x\",\"group\":\"y\"}\n"), 0644); err != nil {
			t.Fatalf("seed %s: %v", label, err)
		}
		paths[label] = path
	}

	if err := PruneOldUsageEvents(30 * 24 * time.Hour); err != nil {
		t.Fatalf("PruneOldUsageEvents: %v", err)
	}

	if _, err := os.Stat(paths["old"]); !os.IsNotExist(err) {
		t.Errorf("expected -45d file removed, stat err = %v", err)
	}
	for _, label := range []string{"edge", "recent", "today"} {
		if _, err := os.Stat(paths[label]); err != nil {
			t.Errorf("expected %s file kept, stat err = %v", label, err)
		}
	}
}

func TestPruneIgnoresUnrelatedFiles(t *testing.T) {
	dir := withTempConfigDir(t)

	// Files that must survive prune regardless of age — they don't match the
	// rotation pattern.
	survivors := []string{
		"config.json",
		"events.jsonl",
		"usage.jsonl", // legacy single file — migration owns it, not prune
		"random.txt",
	}
	for _, name := range survivors {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x\n"), 0644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	// Plus an old per-day file that SHOULD be removed.
	old := time.Now().Add(-90 * 24 * time.Hour)
	oldPath := filepath.Join(dir, "usage-"+old.Format(usageDateLayout)+".jsonl")
	if err := os.WriteFile(oldPath, []byte("{}\n"), 0644); err != nil {
		t.Fatalf("seed old: %v", err)
	}

	if err := PruneOldUsageEvents(30 * 24 * time.Hour); err != nil {
		t.Fatalf("PruneOldUsageEvents: %v", err)
	}

	for _, name := range survivors {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("prune incorrectly removed unrelated file %s: %v", name, err)
		}
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("prune did not remove old per-day file, stat err = %v", err)
	}
}

func TestMigrateLegacyUsageFile(t *testing.T) {
	dir := withTempConfigDir(t)

	day1 := time.Date(2026, 4, 1, 9, 0, 0, 0, time.Local)
	day2 := time.Date(2026, 4, 2, 10, 0, 0, 0, time.Local)
	day3 := time.Date(2026, 4, 3, 11, 0, 0, 0, time.Local)
	legacy := []UsageEvent{
		{TS: day1, Domain: "youtube.com", Group: "social", Kind: KindDNS},
		{TS: day1.Add(1 * time.Hour), Domain: "reddit.com", Group: "social"},
		{TS: day2, Domain: "youtube.com", Group: "social", Kind: KindForeground},
		{TS: day3, Domain: "reddit.com", Group: "social", Kind: KindDNS},
	}

	legacyPath := filepath.Join(dir, "usage.jsonl")
	f, err := os.Create(legacyPath)
	if err != nil {
		t.Fatalf("create legacy: %v", err)
	}
	enc := json.NewEncoder(f)
	for _, e := range legacy {
		if err := enc.Encode(e); err != nil {
			t.Fatalf("encode legacy event: %v", err)
		}
	}
	f.Close()

	if err := MigrateLegacyUsageFile(); err != nil {
		t.Fatalf("MigrateLegacyUsageFile: %v", err)
	}

	// Legacy file should be gone.
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Errorf("legacy file should be removed, stat err = %v", err)
	}

	// Three per-day files should exist with the right contents.
	files, err := listUsageFiles()
	if err != nil {
		t.Fatalf("listUsageFiles after migrate: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 per-day files, got %d: %v", len(files), files)
	}

	gotEvents, err := ReadUsageEventsSince(time.Time{})
	if err != nil {
		t.Fatalf("ReadUsageEventsSince: %v", err)
	}
	if len(gotEvents) != len(legacy) {
		t.Fatalf("event count after migration: got %d, want %d", len(gotEvents), len(legacy))
	}
	// Compare ignoring slice order — events from each day file are appended in
	// sequence, and the daily map iteration order isn't guaranteed.
	sort.Slice(gotEvents, func(i, j int) bool { return gotEvents[i].TS.Before(gotEvents[j].TS) })
	sort.Slice(legacy, func(i, j int) bool { return legacy[i].TS.Before(legacy[j].TS) })
	for i := range legacy {
		if !gotEvents[i].TS.Equal(legacy[i].TS) || gotEvents[i].Domain != legacy[i].Domain {
			t.Errorf("event %d: got %+v, want %+v", i, gotEvents[i], legacy[i])
		}
	}

	// Idempotency: running migrate again on a clean state must be a no-op.
	if err := MigrateLegacyUsageFile(); err != nil {
		t.Fatalf("MigrateLegacyUsageFile (second run): %v", err)
	}
	files2, err := listUsageFiles()
	if err != nil {
		t.Fatalf("listUsageFiles after second migrate: %v", err)
	}
	if len(files2) != 3 {
		t.Errorf("second migrate altered file count: was 3, now %d", len(files2))
	}
}
