package admissions

import "testing"

func TestTimelineEventISODates(t *testing.T) {
	cases := []struct {
		name      string
		event     TimelineEvent
		wantStart string
		wantEnd   string
	}{
		{"blank start means no event", TimelineEvent{StartDate: "-", EndDate: "-"}, "", ""},
		{"single day falls back to start", TimelineEvent{StartDate: "2026-12-11", EndDate: "-"}, "2026-12-11", "2026-12-11"},
		{"range keeps both ends", TimelineEvent{StartDate: "2026-10-20", EndDate: "2026-10-27"}, "2026-10-20", "2026-10-27"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotStart, gotEnd := timelineEventISODates(tc.event)
			if gotStart != tc.wantStart || gotEnd != tc.wantEnd {
				t.Fatalf("timelineEventISODates(%+v) = (%q, %q), want (%q, %q)", tc.event, gotStart, gotEnd, tc.wantStart, tc.wantEnd)
			}
		})
	}
}

func TestDiffTimelineChanges(t *testing.T) {
	before := []TimelineEvent{
		{SortOrder: 1, Name: "網路報名", StartDate: "2026-10-20", EndDate: "2026-10-27"},
		{SortOrder: 2, Name: "正取生報到截止", StartDate: "2026-12-23", EndDate: "-"},
	}
	after := []TimelineEvent{
		{SortOrder: 1, Name: "網路報名", StartDate: "2026-10-20", EndDate: "2026-10-27"}, // unchanged
		{SortOrder: 2, Name: "正取生報到截止", StartDate: "2026-12-24", EndDate: "-"},     // admin corrected the date
		{SortOrder: 3, Name: "備取生遞補作業截止", StartDate: "2027-03-03", EndDate: "-"},  // brand new row, nothing to sync yet
	}

	// Both the corrected row (sort_order 2) and the brand-new row (sort_order
	// 3, which has no "before" to compare against) are reported — a brand
	// new row is harmless to include since SyncEventDates finds zero linked
	// accounts for it and no-ops.
	changes := diffTimelineChanges("116-005-001", before, after)

	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d: %+v", len(changes), changes)
	}
	if changes[0].SortOrder != 2 || changes[0].ISOStart != "2026-12-24" {
		t.Fatalf("unexpected change[0]: %+v", changes[0])
	}
	if changes[1].SortOrder != 3 || changes[1].ISOStart != "2027-03-03" {
		t.Fatalf("unexpected change[1]: %+v", changes[1])
	}
}

// A timeline row moving off the old, buggy end-date-only convention (blank
// start_date, a set end_date — see the formatTimelineEventDate fix earlier
// this session) onto the corrected start_date-only convention is reported
// as a change even when the real-world date didn't move: before the fix,
// the frontend couldn't compute an isoStart for that row at all, so it was
// never addable to a calendar — this now becomes addable for the first
// time and should sync, not be treated as a no-op.
func TestDiffTimelineChangesCatchesConventionFix(t *testing.T) {
	before := []TimelineEvent{{SortOrder: 2, Name: "正取生報到截止", StartDate: "-", EndDate: "2026-12-23"}}
	after := []TimelineEvent{{SortOrder: 2, Name: "正取生報到截止", StartDate: "2026-12-23", EndDate: "-"}}

	changes := diffTimelineChanges("116-005-001", before, after)

	if len(changes) != 1 || changes[0].ISOStart != "2026-12-23" {
		t.Fatalf("expected the now-addable event to be reported, got %+v", changes)
	}
}

func TestDiffTimelineChangesDetectsActualDateMove(t *testing.T) {
	before := []TimelineEvent{{SortOrder: 5, Name: "面試時間", StartDate: "2026-11-21", EndDate: "-"}}
	after := []TimelineEvent{{SortOrder: 5, Name: "面試時間", StartDate: "2026-11-28", EndDate: "-"}}

	changes := diffTimelineChanges("116-032-005", before, after)

	if len(changes) != 1 || changes[0].ISOStart != "2026-11-28" {
		t.Fatalf("expected the moved date to be reported, got %+v", changes)
	}
}
