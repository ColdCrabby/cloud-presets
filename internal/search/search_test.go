package search

import (
	"reflect"
	"testing"
)

func records() []Record {
	return []Record{
		{ID: "prusa-mk4", Type: "printer", Name: "Prusa MK4", Vendor: "prusa", Model: "MK4"},
		{ID: "prusa-mini", Type: "printer", Name: "Prusa Mini+", Vendor: "prusa", Model: "Mini+"},
		{ID: "bambu-x1c", Type: "printer", Name: "Bambu Lab X1 Carbon", Vendor: "bambulab", Model: "X1 Carbon"},
		{ID: "prusament-pla", Type: "filament", Name: "Prusament PLA", Vendor: "prusa", Material: "PLA"},
		{ID: "bambu-abs", Type: "filament", Name: "Bambu ABS", Vendor: "bambulab", Material: "ABS"},
	}
}

func ids(results []Result) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.Record.ID
	}
	return out
}

func TestBrowseReturnsAllSortedByName(t *testing.T) {
	got := ids(Search(records(), Query{}))
	want := []string{"bambu-abs", "bambu-x1c", "prusa-mk4", "prusa-mini", "prusament-pla"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("browse order = %v, want %v", got, want)
	}
	for _, r := range Search(records(), Query{}) {
		if r.Match != nil {
			t.Errorf("browse result %s carries a match; want nil", r.Record.ID)
		}
	}
}

func TestFilterByType(t *testing.T) {
	got := ids(Search(records(), Query{Type: "filament"}))
	want := []string{"bambu-abs", "prusament-pla"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("type filter = %v, want %v", got, want)
	}
}

func TestFilterByVendorAndMaterialCaseInsensitive(t *testing.T) {
	got := ids(Search(records(), Query{Vendor: "PRUSA"}))
	want := []string{"prusa-mk4", "prusa-mini", "prusament-pla"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("vendor filter = %v, want %v", got, want)
	}

	got = ids(Search(records(), Query{Material: "pla"}))
	want = []string{"prusament-pla"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("material filter = %v, want %v", got, want)
	}
}

func TestQueryOnlyReturnsMatches(t *testing.T) {
	results := Search(records(), Query{Text: "bambu"})
	got := ids(results)
	want := []string{"bambu-abs", "bambu-x1c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("query 'bambu' = %v, want %v", got, want)
	}
	for _, r := range results {
		if r.Match == nil {
			t.Errorf("result %s has no match info", r.Record.ID)
		}
	}
}

func TestQueryCombinesWithFilters(t *testing.T) {
	got := ids(Search(records(), Query{Text: "prusa", Type: "printer"}))
	want := []string{"prusa-mk4", "prusa-mini"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("query+filter = %v, want %v", got, want)
	}
}

func TestContiguousMatchRangeAscii(t *testing.T) {
	recs := []Record{{ID: "x", Type: "printer", Name: "Prusa MK4", Vendor: "prusa"}}
	results := Search(recs, Query{Text: "mk4"})
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	m := results[0].Match
	if m == nil || m.Field != "name" {
		t.Fatalf("match = %+v, want field name", m)
	}
	// "Prusa MK4": M at index 6, K4 through 8 => [6,9) in UTF-16 (all BMP).
	want := []MatchRange{{6, 9}}
	if !reflect.DeepEqual(m.Ranges, want) {
		t.Fatalf("ranges = %v, want %v", m.Ranges, want)
	}
}

func TestMatchRangeIsUTF16OffsetsNonAscii(t *testing.T) {
	// Name has an astral emoji (💡, one rune = two UTF-16 code units) before the
	// match, so a byte- or rune-indexed range would misplace the highlight.
	recs := []Record{{ID: "x", Type: "filament", Name: "💡PLA", Vendor: "acme"}}
	results := Search(recs, Query{Text: "pla"})
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	m := results[0].Match
	// 💡 occupies UTF-16 offsets [0,2); "PLA" is [2,5).
	want := []MatchRange{{2, 5}}
	if m == nil || !reflect.DeepEqual(m.Ranges, want) {
		t.Fatalf("ranges = %+v, want %v", m, want)
	}
}

func TestSubsequenceMatchProducesSplitRanges(t *testing.T) {
	recs := []Record{{ID: "x", Type: "printer", Name: "Bambu Lab X1 Carbon", Vendor: "bambulab"}}
	// "bx1" appears as a subsequence in "Bambu Lab X1" (name), not contiguous.
	results := Search(recs, Query{Text: "bx1"})
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	m := results[0].Match
	if m == nil {
		t.Fatal("no match")
	}
	if len(m.Ranges) < 2 {
		t.Fatalf("expected split ranges for a subsequence match, got %v", m.Ranges)
	}
}

func TestContiguousOutranksSubsequence(t *testing.T) {
	recs := []Record{
		{ID: "sub", Type: "printer", Name: "Pxrxuxsxa", Vendor: "v"}, // subsequence of "prusa"
		{ID: "contig", Type: "printer", Name: "Prusa", Vendor: "v"},  // contiguous
	}
	got := ids(Search(recs, Query{Text: "prusa"}))
	if got[0] != "contig" {
		t.Fatalf("contiguous match should rank first; got %v", got)
	}
}

func TestNoMatchExcludesRecord(t *testing.T) {
	results := Search(records(), Query{Text: "zzzzz"})
	if len(results) != 0 {
		t.Fatalf("want no results, got %v", ids(results))
	}
}

func TestDeterministicOrderStable(t *testing.T) {
	a := ids(Search(records(), Query{Text: "prusa"}))
	b := ids(Search(records(), Query{Text: "prusa"}))
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("identical queries returned different orders: %v vs %v", a, b)
	}
}
