package index

import (
	"regexp"
	"strings"
)

// artistSepRE matches the conservative separators used between collaborator
// names. Slash and ampersand work with or without spaces (tags contain both
// "Jay-Z / Linkin Park" and "Thomas/БИ-2/Сплин"), while word-like separators
// require surrounding spaces so ordinary names never break apart.
var artistSepRE = regexp.MustCompile(`(?i)\s*[/;]\s*|\s*&\s*|\s+(?:feat\.?|ft\.?|x|и)\s+`)

// SplitArtistParts splits a raw artist label on conservative separators and
// drops empty pieces. It performs no validation — use SplitArtists for
// grouping, where the known-artist guard decides whether a split is real.
func SplitArtistParts(value string) []string {
	pieces := artistSepRE.Split(value, -1)
	out := make([]string, 0, len(pieces))
	for _, piece := range pieces {
		if piece = strings.TrimSpace(piece); piece != "" {
			out = append(out, piece)
		}
	}
	return out
}

// SplitArtists returns the artist segments a track should be listed under.
//
// A label is only split when it yields at least two segments and at least one
// of them already exists as an artist in known. Real solo/band names that
// happen to contain a separator ("Simon & Garfunkel", "Earth, Wind & Fire",
// "Король и Шут") have no such segment and stay intact, while genuine
// collaborations ("Linkin Park & Jay-Z", "Thomas/БИ-2/Сплин") resolve to each
// performer. Everything else falls back to the trimmed original label.
func SplitArtists(value string, known map[string]struct{}) []string {
	whole := strings.TrimSpace(value)
	segments := SplitArtistParts(value)
	if len(segments) < 2 {
		return []string{whole}
	}
	for _, segment := range segments {
		if _, ok := known[normName(segment)]; ok {
			return segments
		}
	}
	return []string{whole}
}

// CanonicalArtist returns the single artist a composite label groups under:
// the first segment that exists as its own artist, otherwise the first
// segment. For a label that is not split this is the label itself, so album
// grouping never fragments a record across several entries.
func CanonicalArtist(artist string, known map[string]struct{}) string {
	segments := SplitArtists(artist, known)
	if len(segments) == 0 {
		return strings.TrimSpace(artist)
	}
	for _, segment := range segments {
		if _, ok := known[normName(segment)]; ok {
			return segment
		}
	}
	return segments[0]
}

// artistKeyNames lists the artist index keys a track belongs to: its raw
// label first, then every collaborator segment. Keys are normalized and
// deduplicated; names keep the casing seen in the tag.
func artistKeyNames(artist string, known map[string]struct{}) [][2]string {
	out := make([][2]string, 0, 3)
	seen := make(map[string]bool, 3)
	add := func(name string) {
		key := normName(name)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, [2]string{key, name})
	}
	add(strings.TrimSpace(artist))
	for _, segment := range SplitArtists(artist, known) {
		add(segment)
	}
	return out
}
