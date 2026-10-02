package index

import (
	"reflect"
	"testing"

	"github.com/torwin-job/musik/player/internal/config"
	"github.com/torwin-job/musik/player/internal/db"
)

func knownArtists(names ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[normName(name)] = struct{}{}
	}
	return out
}

func TestSplitArtistParts(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"Thomas/БИ-2/Сплин", []string{"Thomas", "БИ-2", "Сплин"}},
		{"Jay-Z / Linkin Park", []string{"Jay-Z", "Linkin Park"}},
		{"Linkin Park feat. Pusha T", []string{"Linkin Park", "Pusha T"}},
		{"X-Ecutioners feat. Mike Shinoda & Mr. Hahn", []string{"X-Ecutioners", "Mike Shinoda", "Mr. Hahn"}},
		{"Linkin Park; Steve Aoki", []string{"Linkin Park", "Steve Aoki"}},
		{"Linkin Park x Steve Aoki", []string{"Linkin Park", "Steve Aoki"}},
		{"Король и Шут", []string{"Король", "Шут"}},
		{"AC/DC", []string{"AC", "DC"}},
		{"Earth, Wind & Fire", []string{"Earth, Wind", "Fire"}},
		{"Scorpions (The Hunters)", []string{"Scorpions (The Hunters)"}},
		{"", []string{}},
		{"   ", []string{}},
	}
	for _, c := range cases {
		if got := SplitArtistParts(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("SplitArtistParts(%q)=%v, want %v", c.in, got, c.want)
		}
	}
}

func TestSplitArtistsGuard(t *testing.T) {
	known := knownArtists("Linkin Park", "БИ-2", "Сплин", "Scorpions", "Король и Шут")
	cases := []struct {
		in   string
		want []string
	}{
		// Genuine collaborations resolve to each performer.
		{"Thomas/БИ-2/Сплин", []string{"Thomas", "БИ-2", "Сплин"}},
		{"Linkin Park & Jay-Z", []string{"Linkin Park", "Jay-Z"}},
		{"Jay-Z/Linkin Park", []string{"Jay-Z", "Linkin Park"}},
		{"Scorpions/M. Kleitman", []string{"Scorpions", "M. Kleitman"}},
		// Solo and band names containing a separator stay whole: neither part
		// exists as an artist of its own.
		{"Король и Шут", []string{"Король и Шут"}},
		{"Simon & Garfunkel", []string{"Simon & Garfunkel"}},
		{"Earth, Wind & Fire", []string{"Earth, Wind & Fire"}},
		// No separator and empty labels are returned untouched.
		{"Scorpions (The Hunters)", []string{"Scorpions (The Hunters)"}},
		{"Linkin Park", []string{"Linkin Park"}},
		{"", []string{""}},
	}
	for _, c := range cases {
		if got := SplitArtists(c.in, known); !reflect.DeepEqual(got, c.want) {
			t.Errorf("SplitArtists(%q)=%v, want %v", c.in, got, c.want)
		}
	}
	if got := SplitArtists("Linkin Park & Jay-Z", nil); !reflect.DeepEqual(got, []string{"Linkin Park & Jay-Z"}) {
		t.Errorf("nil known should not split, got %v", got)
	}
}

func TestCanonicalArtist(t *testing.T) {
	known := knownArtists("Linkin Park", "БИ-2", "Сплин", "Король и Шут")
	cases := []struct {
		in   string
		want string
	}{
		{"Jay-Z / Linkin Park", "Linkin Park"},
		{"Thomas/БИ-2/Сплин", "БИ-2"},
		{"Король и Шут", "Король и Шут"},
		{"Сплин", "Сплин"},
		{"", ""},
	}
	for _, c := range cases {
		if got := CanonicalArtist(c.in, known); got != c.want {
			t.Errorf("CanonicalArtist(%q)=%q, want %q", c.in, got, c.want)
		}
	}
}

func TestLoadIndexesCollaboratorSegments(t *testing.T) {
	idx := New(config.Config{})
	rows := []db.TrackRow{
		{ID: 1, Artist: "Linkin Park", Album: "Meteora", Embedding: Float32Bytes([]float32{1, 0}), Dim: 2},
		{ID: 2, Artist: "Linkin Park & Jay-Z", Album: "Collision Course", Embedding: Float32Bytes([]float32{0.8, 0.2}), Dim: 2},
		{ID: 3, Artist: "Thomas/БИ-2/Сплин", Album: "Fellini 2001 Tour", Embedding: Float32Bytes([]float32{0, 1}), Dim: 2},
		{ID: 4, Artist: "Король и Шут", Album: "Ангел-Демон", Embedding: Float32Bytes([]float32{-1, 0}), Dim: 2},
		{ID: 5, Artist: "БИ-2", Album: "Город золота", Embedding: Float32Bytes([]float32{0.5, 0.5}), Dim: 2},
		{ID: 6, Artist: "Сплин", Album: "Гранатовый альбом", Embedding: Float32Bytes([]float32{-0.5, 0.5}), Dim: 2},
	}
	if err := idx.Load(rows); err != nil {
		t.Fatal(err)
	}
	if got := idx.RowsForArtist("Linkin Park"); len(got) != 2 {
		t.Fatalf("Linkin Park rows=%v, want track 1 and the collaboration", got)
	}
	if got := idx.RowsForArtist("Jay-Z"); len(got) != 1 || got[0] != 1 {
		t.Fatalf("Jay-Z rows=%v, want [1]", got)
	}
	if got := idx.RowsForArtist("Сплин"); len(got) != 2 {
		t.Fatalf("Сплин rows=%v, want solo record plus the collaboration", got)
	}
	if got := idx.RowsForArtist("БИ-2"); len(got) != 2 {
		t.Fatalf("БИ-2 rows=%v, want solo record plus the collaboration", got)
	}
	if got := idx.RowsForArtist("Король"); len(got) != 0 {
		t.Fatalf("Король rows=%v, want none (band name must not split)", got)
	}
	if got := idx.RowsForAlbum("Сплин", "Fellini 2001 Tour"); len(got) != 1 {
		t.Fatalf("album on artist page=%v, want the collaboration", got)
	}
	if got := idx.RowsForAlbum("Linkin Park", "Collision Course"); len(got) != 1 {
		t.Fatalf("album on artist page=%v, want the collaboration", got)
	}
}
