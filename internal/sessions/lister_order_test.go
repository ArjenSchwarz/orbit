package sessions

import (
	"slices"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// extendedSourceOrder is the tie-break order of requirement 4.4.
var extendedSourceOrder = []string{SourceClaude, SourceCopilot, SourceCodex, SourceKiroCLI, SourceKiroIDE}

func TestSortSessionsExtended_Example(t *testing.T) {
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	list := []SessionInfo{
		{ID: "b", Source: SourceKiroIDE, CreatedAt: base},
		{ID: "late", Source: SourceClaude, CreatedAt: base.Add(time.Hour)},
		{ID: "a", Source: SourceKiroIDE, CreatedAt: base},
		{ID: "z", Source: SourceClaude, CreatedAt: base},
		{ID: "unknown", Source: SourceCodex},
	}

	sortSessionsExtended(list)

	var got []string
	for _, s := range list {
		got = append(got, s.Source+"/"+s.ID)
	}
	want := []string{"codex/unknown", "claude/z", "kiro-ide/a", "kiro-ide/b", "claude/late"}
	if !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestPropertySortSessionsExtended(t *testing.T) {
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	session := rapid.Custom(func(t *rapid.T) SessionInfo {
		var created time.Time
		if offset := rapid.IntRange(-1, 3).Draw(t, "offset"); offset >= 0 {
			created = base.Add(time.Duration(offset) * time.Minute)
		}
		return SessionInfo{
			ID:        rapid.SampledFrom([]string{"a", "b", "c"}).Draw(t, "id"),
			Source:    rapid.SampledFrom(extendedSourceOrder).Draw(t, "source"),
			CreatedAt: created,
		}
	})

	rapid.Check(t, func(t *rapid.T) {
		list := rapid.SliceOf(session).Draw(t, "sessions")
		sorted := slices.Clone(list)
		sortSessionsExtended(sorted)

		for i := 1; i < len(sorted); i++ {
			prev, cur := sorted[i-1], sorted[i]
			switch {
			case prev.CreatedAt.IsZero() != cur.CreatedAt.IsZero():
				if !prev.CreatedAt.IsZero() {
					t.Fatalf("known time %v sorted before unknown time at %d", prev.CreatedAt, i)
				}
			case !prev.CreatedAt.Equal(cur.CreatedAt):
				if cur.CreatedAt.Before(prev.CreatedAt) {
					t.Fatalf("times out of order at %d: %v then %v", i, prev.CreatedAt, cur.CreatedAt)
				}
			case prev.Source != cur.Source:
				if slices.Index(extendedSourceOrder, cur.Source) < slices.Index(extendedSourceOrder, prev.Source) {
					t.Fatalf("equal times: source %s sorted before %s", prev.Source, cur.Source)
				}
			case cur.ID < prev.ID:
				t.Fatalf("equal time and source: id %s sorted before %s", prev.ID, cur.ID)
			}
		}

		// The order is total: every permutation of the input sorts to the same keys,
		// so latest (the last element) does not depend on collection order.
		shuffled := slices.Clone(list)
		slices.Reverse(shuffled)
		sortSessionsExtended(shuffled)
		for i := range sorted {
			if compareSessionsExtended(sorted[i], shuffled[i]) != 0 {
				t.Fatalf("position %d differs by input order: %+v vs %+v", i, sorted[i], shuffled[i])
			}
		}
		if len(sorted) > 0 {
			latest := sorted[len(sorted)-1]
			for _, s := range sorted {
				if compareSessionsExtended(s, latest) > 0 {
					t.Fatalf("%+v sorts after latest %+v", s, latest)
				}
			}
		}
	})
}
