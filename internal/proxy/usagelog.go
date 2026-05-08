package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vsangava/sentinel/internal/config"
)

// UsageEvent records a single observation that a domain in a known group was
// touched by the user. Two flavours coexist in the same JSONL log:
//
//   - Kind == "" (legacy) or "dns": one entry per DNS lookup. Aggregated into
//     5-minute buckets — feeds the existing used_minutes / quota signal.
//   - Kind == "foreground": one entry per scheduler tick where the active
//     browser tab matched a configured group. Aggregated into 1-minute buckets
//     — feeds foreground_minutes. macOS-only.
//
// The empty-string default for Kind is intentional — it keeps every entry that
// existed before foreground tracking was introduced parsing as DNS without a
// migration step.
type UsageEvent struct {
	TS     time.Time `json:"ts"`
	Domain string    `json:"domain"`
	Group  string    `json:"group"`
	Kind   string    `json:"kind,omitempty"`
}

// Usage event Kind constants. Empty Kind on an existing event is equivalent to
// KindDNS for backwards compatibility.
const (
	KindDNS        = "dns"
	KindForeground = "foreground"
)

// IsDNSKind reports whether a UsageEvent should count toward DNS-bucket usage.
// Treats the legacy empty Kind as DNS so pre-feature events keep aggregating.
func (e UsageEvent) IsDNSKind() bool {
	return e.Kind == "" || e.Kind == KindDNS
}

// usageDateLayout is the date stamp used in per-day log filenames. Local time
// matches how ComputeGroup*Minutes derives day boundaries from t.Location().
const usageDateLayout = "2006-01-02"

// legacyUsageFileName is the pre-rotation single-file log. Kept as a string
// constant so the migration path and the prune path agree on what to leave
// alone (prune must NOT delete this file — migration is responsible).
const legacyUsageFileName = "usage.jsonl"

// usageFilePathForDate returns the per-day log path for the date portion of t,
// in t's own timezone. Filename shape: usage-YYYY-MM-DD.jsonl.
func usageFilePathForDate(t time.Time) string {
	return filepath.Join(config.ConfigDir(), "usage-"+t.Format(usageDateLayout)+".jsonl")
}

// parseUsageFileDate extracts the date encoded in a per-day log filename.
// Returns ok=false if the name doesn't match the rotation pattern — callers
// use this to skip unrelated files (including the legacy single file).
func parseUsageFileDate(name string) (time.Time, bool) {
	const prefix = "usage-"
	const suffix = ".jsonl"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return time.Time{}, false
	}
	stamp := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
	d, err := time.ParseInLocation(usageDateLayout, stamp, time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return d, true
}

// listUsageFiles returns per-day log files in the config dir, oldest-first by
// the date encoded in the filename. Files whose names don't match the
// rotation pattern (including the legacy usage.jsonl) are skipped.
func listUsageFiles() ([]string, error) {
	dir := config.ConfigDir()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	type dated struct {
		path string
		when time.Time
	}
	var found []dated
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		when, ok := parseUsageFileDate(e.Name())
		if !ok {
			continue
		}
		found = append(found, dated{path: filepath.Join(dir, e.Name()), when: when})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].when.Before(found[j].when) })
	out := make([]string, len(found))
	for i, f := range found {
		out[i] = f.path
	}
	return out, nil
}

// AppendUsageEvent appends a single usage event to the per-day JSONL log. The
// file is opened in append mode and closed on each call — appends from the DNS
// proxy and the scheduler are atomic at the kernel level for writes ≤PIPE_BUF
// so no file-level lock is needed. Best-effort.
func AppendUsageEvent(e UsageEvent) error {
	path := usageFilePathForDate(e.TS)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(e)
}

// ReadUsageEventsSince returns all usage events with TS strictly after since,
// across every per-day log on disk. Files outside the requested window are
// skipped at the directory-listing layer so the read cost scales with the
// requested range, not with retention. A zero since means no lower bound.
func ReadUsageEventsSince(since time.Time) ([]UsageEvent, error) {
	files, err := listUsageFiles()
	if err != nil {
		return nil, err
	}

	// Cheap directory-level skip: drop files whose date is strictly before the
	// calendar day containing `since`. The per-event TS check below is still
	// authoritative for the partial-day boundary.
	var sinceDay time.Time
	if !since.IsZero() {
		sinceDay = time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, since.Location())
	}

	var events []UsageEvent
	for _, path := range files {
		if !since.IsZero() {
			when, ok := parseUsageFileDate(filepath.Base(path))
			if ok && when.Before(sinceDay) {
				continue
			}
		}
		fileEvents, err := readUsageFile(path, since)
		if err != nil {
			return nil, err
		}
		events = append(events, fileEvents...)
	}
	return events, nil
}

// readUsageFile decodes one per-day log, applying the per-event since filter.
// Malformed lines are skipped silently — the log is best-effort and a single
// torn write must not poison the entire range.
func readUsageFile(path string, since time.Time) ([]UsageEvent, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var events []UsageEvent
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e UsageEvent
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		if !since.IsZero() && !e.TS.After(since) {
			continue
		}
		events = append(events, e)
	}
	return events, scanner.Err()
}

// PruneOldUsageEvents removes per-day log files whose date is strictly older
// than now-maxAge. Per-day rotation makes this an O(deleted-files) `unlink`
// rather than a full-file rewrite, and there's no append-vs-prune race
// because today's file is never a prune candidate.
func PruneOldUsageEvents(maxAge time.Duration) error {
	files, err := listUsageFiles()
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-maxAge)
	cutoffDay := time.Date(cutoff.Year(), cutoff.Month(), cutoff.Day(), 0, 0, 0, 0, cutoff.Location())
	for _, path := range files {
		when, ok := parseUsageFileDate(filepath.Base(path))
		if !ok {
			continue
		}
		if when.Before(cutoffDay) {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove %s: %w", path, err)
			}
		}
	}
	return nil
}

// MigrateLegacyUsageFile splits a pre-rotation usage.jsonl into per-day files
// and removes the original on success. No-op if the legacy file doesn't exist
// (which is the steady state on any machine that's already rotated, or any
// fresh install). Idempotent — re-running after success finds nothing to do.
//
// Failure mode is conservative: if any per-day write fails, the legacy file is
// left in place so the daemon can re-attempt on next start. Per-day files
// already written stay; AppendUsageEvent's O_APPEND semantics mean a partial
// migration plus normal operation will not duplicate or lose events.
func MigrateLegacyUsageFile() error {
	legacyPath := filepath.Join(config.ConfigDir(), legacyUsageFileName)
	f, err := os.Open(legacyPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	perDay := make(map[string][]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		var e UsageEvent
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		key := e.TS.Format(usageDateLayout)
		perDay[key] = append(perDay[key], string(line))
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan legacy usage log: %w", err)
	}

	for stamp, lines := range perDay {
		path := filepath.Join(config.ConfigDir(), "usage-"+stamp+".jsonl")
		if err := appendLines(path, lines); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}

	f.Close()
	if err := os.Remove(legacyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove legacy usage log: %w", err)
	}
	return nil
}

func appendLines(path string, lines []string) error {
	out, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer out.Close()
	for _, l := range lines {
		if _, err := out.WriteString(l + "\n"); err != nil {
			return err
		}
	}
	return nil
}

// bucketKey returns the 5-minute bucket key for a timestamp.
// Queries within the same 5-minute window collapse to one bucket.
func bucketKey(t time.Time) int64 {
	return t.Unix() / 300
}

// ComputeGroupUsageMinutes returns the number of minutes a group was actively
// used on the calendar day containing t, derived from the provided events.
// Usage is measured in distinct 5-minute buckets × 5 minutes. Only DNS-kind
// events count — foreground events live alongside in the same log but are
// aggregated separately via ComputeGroupForegroundMinutes.
func ComputeGroupUsageMinutes(events []UsageEvent, group string, t time.Time) int {
	dayStart := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	dayEnd := dayStart.Add(24 * time.Hour)

	buckets := make(map[int64]struct{})
	for _, e := range events {
		if !e.IsDNSKind() {
			continue
		}
		if e.Group != group {
			continue
		}
		if e.TS.Before(dayStart) || !e.TS.Before(dayEnd) {
			continue
		}
		buckets[bucketKey(e.TS)] = struct{}{}
	}
	return len(buckets) * 5
}

// ComputeAllGroupUsageMinutes returns ComputeGroupUsageMinutes for every group
// in groups, for the calendar day containing t.
func ComputeAllGroupUsageMinutes(events []UsageEvent, groups []string, t time.Time) map[string]int {
	result := make(map[string]int, len(groups))
	for _, g := range groups {
		result[g] = ComputeGroupUsageMinutes(events, g, t)
	}
	return result
}

// minuteBucketKey returns the 1-minute bucket for a timestamp. Foreground
// observations are minute-granular by construction (one per scheduler tick),
// so 5-minute bucketing would only obscure the signal.
func minuteBucketKey(t time.Time) int64 {
	return t.Unix() / 60
}

// ComputeGroupForegroundMinutes returns minutes spent with the foreground
// browser tab on a domain in `group`, for the calendar day containing t.
// Counts distinct 1-minute buckets — duplicate observations within the same
// minute (shouldn't happen, but harmless) collapse to a single tick.
func ComputeGroupForegroundMinutes(events []UsageEvent, group string, t time.Time) int {
	dayStart := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	dayEnd := dayStart.Add(24 * time.Hour)

	buckets := make(map[int64]struct{})
	for _, e := range events {
		if e.Kind != KindForeground {
			continue
		}
		if e.Group != group {
			continue
		}
		if e.TS.Before(dayStart) || !e.TS.Before(dayEnd) {
			continue
		}
		buckets[minuteBucketKey(e.TS)] = struct{}{}
	}
	return len(buckets)
}
