package search

import (
	"sort"
	"strings"
	"unicode"
)

// Record is a single searchable preset summary held in the catalog's in-memory
// index. It carries only the short text fields search ranks over plus the
// identity a result row renders, never the full slicing parameters.
//
// Model and Material are empty when they do not apply to the record's type
// (process presets have neither; printers have no material; filaments have no
// model), and the API renders those empty strings as JSON null.
type Record struct {
	ID       string
	Type     string // printer | filament | process
	Name     string
	Vendor   string
	Model    string
	Material string
	Spec     string
}

// Query carries the search filters. Text is the fuzzy query; the remaining
// fields are exact (case-insensitive) filters. Any empty field is ignored.
type Query struct {
	Text     string
	Type     string
	Vendor   string
	Material string
}

// MatchRange is a [start, end) pair of UTF-16 code-unit offsets into the
// original, unnormalised field value, matching the wire contract the frontends
// consume for highlighting.
type MatchRange [2]int

// Match reports which field the query matched and the offsets within that
// field's value that matched, so the client highlights exactly what ranked.
type Match struct {
	Field  string
	Ranges []MatchRange
}

// Result is one ranked record. Match is nil when browsing (no text query), so
// callers can omit highlighting entirely for a plain browse.
type Result struct {
	Record Record
	Match  *Match
	Score  int
}

// Search filters records and, when a text query is present, ranks the matches
// by fuzzy score. The result order is deterministic: matches sort by descending
// score and then by name and id, so equal scores never reorder between
// identical requests (a requirement for stable cursor pagination). Browsing
// (empty text) returns every filtered record ordered by name then id.
func Search(records []Record, q Query) []Result {
	var out []Result
	if q.Text == "" {
		for _, rec := range records {
			if !passesFilters(rec, q) {
				continue
			}
			out = append(out, Result{Record: rec})
		}
		sort.SliceStable(out, func(i, j int) bool {
			return lessByName(out[i].Record, out[j].Record)
		})
		return out
	}

	query := lowerRunes(q.Text)
	for _, rec := range records {
		if !passesFilters(rec, q) {
			continue
		}
		if m, score, ok := bestMatch(rec, query); ok {
			out = append(out, Result{Record: rec, Match: m, Score: score})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return lessByName(out[i].Record, out[j].Record)
	})
	return out
}

// lessByName is the deterministic tie-breaker: name first, id to settle
// duplicate names.
func lessByName(a, b Record) bool {
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	return a.ID < b.ID
}

// passesFilters applies the exact (case-insensitive) type, vendor and material
// filters. An empty filter field matches everything.
func passesFilters(rec Record, q Query) bool {
	if q.Type != "" && !strings.EqualFold(rec.Type, q.Type) {
		return false
	}
	if q.Vendor != "" && !strings.EqualFold(rec.Vendor, q.Vendor) {
		return false
	}
	if q.Material != "" && !strings.EqualFold(rec.Material, q.Material) {
		return false
	}
	return true
}

// searchField pairs a record field's name and value with the weight its matches
// carry, so a hit in the name outranks an equally-close hit in the material.
type searchField struct {
	name   string
	value  string
	weight int
}

// bestMatch finds the highest-scoring field match for query across the record's
// searchable fields and returns it as a *Match plus the combined score.
func bestMatch(rec Record, query []rune) (*Match, int, bool) {
	fields := []searchField{
		{"name", rec.Name, 3},
		{"vendor", rec.Vendor, 2},
		{"model", rec.Model, 1},
		{"material", rec.Material, 1},
	}

	var best *Match
	bestScore := 0
	for _, f := range fields {
		if f.value == "" {
			continue
		}
		ranges, score, ok := matchField(f.value, query)
		if !ok {
			continue
		}
		score += f.weight
		if best == nil || score > bestScore {
			best = &Match{Field: f.name, Ranges: ranges}
			bestScore = score
		}
	}
	if best == nil {
		return nil, 0, false
	}
	return best, bestScore, true
}

// matchField matches query against a single field value and, on a hit, returns
// the matched ranges as UTF-16 offsets into the original value plus a score.
//
// It prefers a contiguous substring hit (higher score, earlier and prefix hits
// score highest) and falls back to a Sublime-style subsequence match. Matching
// is case-insensitive via per-rune folding, which keeps a 1:1 rune mapping to
// the original value so the reported offsets land on the original characters —
// the trap called out in docs/api-surface.md, where a range computed over
// case-folded or normalised text silently misplaces highlights on non-ASCII
// text.
func matchField(value string, query []rune) ([]MatchRange, int, bool) {
	runes := []rune(value)
	folded := lowerRunesOf(runes)
	offsets := utf16Offsets(runes)

	if start, ok := indexOfSubsequenceContiguous(folded, query); ok {
		end := start + len(query)
		r := MatchRange{offsets[start], offsets[end]}
		// Earlier matches rank higher; a prefix match ranks highest of all.
		score := 100 - min(start, 50)
		if start == 0 {
			score += 50
		}
		return []MatchRange{r}, score, true
	}

	matched, ok := subsequenceIndices(folded, query)
	if !ok {
		return nil, 0, false
	}
	ranges := groupRanges(matched, offsets)
	// Fewer, tighter runs (closer to contiguous) score higher.
	score := 40 - min(len(ranges)-1, 39)
	return ranges, score, true
}

// indexOfSubsequenceContiguous returns the earliest index where query appears
// as a contiguous run in folded.
func indexOfSubsequenceContiguous(folded, query []rune) (int, bool) {
	if len(query) == 0 || len(query) > len(folded) {
		return 0, false
	}
	for i := 0; i+len(query) <= len(folded); i++ {
		hit := true
		for j := range query {
			if folded[i+j] != query[j] {
				hit = false
				break
			}
		}
		if hit {
			return i, true
		}
	}
	return 0, false
}

// subsequenceIndices greedily matches query as a subsequence of folded and
// returns the matched rune indices, or ok=false if query is not a subsequence.
func subsequenceIndices(folded, query []rune) ([]int, bool) {
	if len(query) == 0 {
		return nil, false
	}
	matched := make([]int, 0, len(query))
	qi := 0
	for fi := 0; fi < len(folded) && qi < len(query); fi++ {
		if folded[fi] == query[qi] {
			matched = append(matched, fi)
			qi++
		}
	}
	if qi < len(query) {
		return nil, false
	}
	return matched, true
}

// groupRanges collapses consecutive matched rune indices into contiguous ranges
// and converts them to UTF-16 offsets using offsets.
func groupRanges(matched []int, offsets []int) []MatchRange {
	var ranges []MatchRange
	start := matched[0]
	prev := matched[0]
	for _, idx := range matched[1:] {
		if idx == prev+1 {
			prev = idx
			continue
		}
		ranges = append(ranges, MatchRange{offsets[start], offsets[prev+1]})
		start = idx
		prev = idx
	}
	ranges = append(ranges, MatchRange{offsets[start], offsets[prev+1]})
	return ranges
}

// utf16Offsets returns the cumulative UTF-16 code-unit offset before each rune,
// with one extra trailing entry for the end of the string. Offset[i] is where
// rune i begins in UTF-16 space; offset[len] is the total UTF-16 length.
func utf16Offsets(runes []rune) []int {
	offsets := make([]int, len(runes)+1)
	for i, r := range runes {
		width := 1
		if r >= 0x10000 {
			width = 2
		}
		offsets[i+1] = offsets[i] + width
	}
	return offsets
}

// lowerRunes folds a string to lower-case runes, preserving a 1:1 rune mapping
// with the input so offsets computed against it map back to the original.
func lowerRunes(s string) []rune {
	return lowerRunesOf([]rune(s))
}

func lowerRunesOf(runes []rune) []rune {
	out := make([]rune, len(runes))
	for i, r := range runes {
		out[i] = unicode.ToLower(r)
	}
	return out
}
