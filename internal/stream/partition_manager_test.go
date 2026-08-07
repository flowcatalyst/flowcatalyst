package stream

import (
	"testing"
	"time"
)

func TestParsePartitionEnd(t *testing.T) {
	cases := []struct {
		name   string
		parent string
		want   string // exclusive end (RFC3339 date), "" => ok=false
	}{
		{"msg_events_2026_03", "msg_events", "2026-04-01"},
		{"msg_events_2026_12", "msg_events", "2027-01-01"},
		{"msg_events_read_2026_01", "msg_events_read", "2026-02-01"},
		{"msg_events_default", "msg_events", ""},        // not YYYY_MM
		{"msg_events_2026_13", "msg_events", ""},        // bad month
		{"msg_dispatch_jobs_2026_06", "msg_events", ""}, // wrong parent prefix
		{"msg_events", "msg_events", ""},                // no suffix
	}
	for _, c := range cases {
		end, ok := parsePartitionEnd(c.name, c.parent)
		if c.want == "" {
			if ok {
				t.Errorf("parsePartitionEnd(%q,%q) = %s, want ok=false", c.name, c.parent, end)
			}
			continue
		}
		if !ok {
			t.Errorf("parsePartitionEnd(%q,%q) ok=false, want %s", c.name, c.parent, c.want)
			continue
		}
		want, _ := time.Parse("2006-01-02", c.want)
		if !end.Equal(want.UTC()) {
			t.Errorf("parsePartitionEnd(%q) = %s, want %s", c.name, end, want)
		}
	}
}

func TestParsePartitionEndDriveRetention(t *testing.T) {
	// A 2026-01 partition ends 2026-02-01; with a 90-day retention measured
	// from 2026-06-01 the cutoff is ~2026-03-03, so the partition is expired.
	end, ok := parsePartitionEnd("msg_events_2026_01", "msg_events")
	if !ok {
		t.Fatal("expected parse ok")
	}
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	cutoff := now.AddDate(0, 0, -90)
	if end.After(cutoff) {
		t.Errorf("expected 2026-01 partition (end %s) to be <= cutoff %s", end, cutoff)
	}
	// A 2026-05 partition ends 2026-06-01, which is after the cutoff → kept.
	end2, _ := parsePartitionEnd("msg_events_2026_05", "msg_events")
	if !end2.After(cutoff) {
		t.Errorf("expected 2026-05 partition (end %s) to be > cutoff %s", end2, cutoff)
	}
}

// The scheduled-job history tables get their own (shorter) retention window;
// every other partitioned parent keeps the global one. Zero-config: the
// 30-day default applies when the field is unset.
func TestRetentionFor_ScheduledJobTables(t *testing.T) {
	m := &PartitionManager{}
	cfg := m.cfg() // all defaults

	if got := cfg.retentionFor("msg_events"); got != 90 {
		t.Errorf("msg_events retention = %d, want global default 90", got)
	}
	if got := cfg.retentionFor("msg_scheduled_job_instances"); got != 30 {
		t.Errorf("msg_scheduled_job_instances retention = %d, want scheduled-job default 30", got)
	}
	if got := cfg.retentionFor("msg_scheduled_job_instance_logs"); got != 30 {
		t.Errorf("msg_scheduled_job_instance_logs retention = %d, want scheduled-job default 30", got)
	}

	// Explicit overrides win, independently.
	m.Config = PartitionManagerConfig{RetentionDays: 120, ScheduledJobRetentionDays: 7}
	cfg = m.cfg()
	if got := cfg.retentionFor("msg_dispatch_jobs"); got != 120 {
		t.Errorf("msg_dispatch_jobs retention = %d, want 120", got)
	}
	if got := cfg.retentionFor("msg_scheduled_job_instances"); got != 7 {
		t.Errorf("msg_scheduled_job_instances retention = %d, want 7", got)
	}
}
