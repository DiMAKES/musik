package library

import (
	"strings"
	"testing"

	"github.com/torwin-job/musik/player/internal/config"
	"github.com/torwin-job/musik/player/internal/db"
	"github.com/torwin-job/musik/player/internal/index"
)

func testIndex(t *testing.T, rows []db.TrackRow) *index.Index {
	t.Helper()
	idx := index.New(config.Config{})
	if err := idx.Load(rows); err != nil {
		t.Fatal(err)
	}
	return idx
}

func vec(id int64) []byte {
	return index.Float32Bytes([]float32{1, float32(id)})
}

func knownSet(names ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[index.ArtistKey(name)] = struct{}{}
	}
	return out
}

func artistTracks(groups []ArtistGroup, name string) int {
	for _, g := range groups {
		if strings.EqualFold(g.Artist, name) {
			return g.Tracks
		}
	}
	return 0
}

func TestMatchArtistAlbum(t *testing.T) {
	known := knownSet("Massive Attack", "Portishead", "Linkin Park", "БИ-2", "Сплин")
	if !MatchArtistAlbum("Massive Attack", "Mezzanine", " massive attack ", "", known) {
		t.Fatal("artist match should be case-insensitive")
	}
	if MatchArtistAlbum("Massive Attack", "Protection", "Massive Attack", "mezzanine", known) {
		t.Fatal("album mismatch should fail")
	}
	if !MatchArtistAlbum("Portishead", "Dummy", "", "", known) {
		t.Fatal("empty filters should match")
	}
	// A collaboration matches either of its performers.
	if !MatchArtistAlbum("Linkin Park & Jay-Z", "Collision Course", "Jay-Z", "", known) {
		t.Fatal("collaboration should match its second performer")
	}
	if !MatchArtistAlbum("Thomas/БИ-2/Сплин", "Fellini 2001 Tour", "Сплин", "", known) {
		t.Fatal("collaboration should match its second performer")
	}
	// A band name that only looks like a collaboration must not match a part.
	if MatchArtistAlbum("Король и Шут", "Ангел-Демон", "Король", "", known) {
		t.Fatal("band name should not match on a separator part")
	}
	if !MatchArtistAlbum("Король и Шут", "Ангел-Демон", "Король и Шут", "", known) {
		t.Fatal("band name should match as a whole")
	}
}

func TestGroupArtistsAndAlbums(t *testing.T) {
	idx := testIndex(t, []db.TrackRow{
		{ID: 1, Title: "One", Artist: "Massive Attack", Album: "Mezzanine", Embedding: vec(1), Dim: 2, ArtworkPath: "/a.png"},
		{ID: 2, Title: "Two", Artist: "Massive Attack", Album: "Protection", Embedding: vec(2), Dim: 2},
		{ID: 3, Title: "Three", Artist: "Portishead", Album: "Dummy", Embedding: vec(3), Dim: 2},
		{ID: 4, Title: "Four", Artist: "", Album: "", Embedding: vec(4), Dim: 2},
		{ID: 5, Title: "Five", Artist: "Linkin Park", Album: "Meteora", Embedding: vec(5), Dim: 2},
		{ID: 6, Title: "Six", Artist: "Linkin Park & Jay-Z", Album: "Collision Course", Embedding: vec(6), Dim: 2},
		{ID: 7, Title: "Seven", Artist: "Thomas/БИ-2/Сплин", Album: "Fellini 2001 Tour", Embedding: vec(7), Dim: 2},
		{ID: 8, Title: "Eight", Artist: "Король и Шут", Album: "Ангел-Демон", Embedding: vec(8), Dim: 2},
		{ID: 9, Title: "Nine", Artist: "БИ-2", Album: "Город золота", Embedding: vec(9), Dim: 2},
		{ID: 10, Title: "Ten", Artist: "Сплин", Album: "Гранатовый альбом", Embedding: vec(10), Dim: 2},
	})

	artists := GroupArtists(idx)
	// Massive Attack, Portishead, Unknown, Linkin Park, Jay-Z, Thomas, БИ-2,
	// Сплин and Король и Шут — every segment, but no combination label.
	want := []string{
		"Massive Attack", "Portishead", "Unknown", "Linkin Park", "Jay-Z",
		"Thomas", "БИ-2", "Сплин", "Король и Шут",
	}
	if len(artists) != len(want) {
		t.Fatalf("artists=%d, want %d: %+v", len(artists), len(want), artists)
	}
	for _, name := range want {
		if artistTracks(artists, name) == 0 {
			t.Fatalf("missing artist group %q in %+v", name, artists)
		}
	}
	for _, g := range artists {
		if strings.ContainsAny(g.Artist, "&/") {
			t.Fatalf("combination artist group should not exist: %+v", g)
		}
	}
	if n := artistTracks(artists, "Massive Attack"); n != 2 {
		t.Fatalf("Massive Attack tracks=%d, want 2", n)
	}
	covered := false
	for _, g := range artists {
		if g.HasArtwork && strings.EqualFold(g.Artist, "Massive Attack") {
			covered = true
		}
	}
	if !covered {
		t.Fatalf("Massive Attack group should carry the cover: %+v", artists)
	}
	if n := artistTracks(artists, "Linkin Park"); n != 2 {
		t.Fatalf("Linkin Park tracks=%d, want solo record plus collaboration", n)
	}
	if n := artistTracks(artists, "БИ-2"); n != 2 || artistTracks(artists, "Сплин") != 2 {
		t.Fatalf("Би-2/Сплин groups wrong: %+v", artists)
	}
	if n := artistTracks(artists, "Король"); n != 0 {
		t.Fatalf("Король group=%d, want 0 (band name must stay whole)", n)
	}

	albums := GroupAlbums(idx)
	// Mezzanine, Protection, Dummy, Meteora, Collision Course (under Linkin
	// Park), Fellini 2001 Tour (under БИ-2), Ангел-Демон, Город золота and
	// Гранатовый альбом.
	if len(albums) != 9 {
		t.Fatalf("albums=%d, want 9: %+v", len(albums), albums)
	}
	for _, al := range albums {
		if strings.ContainsAny(al.Artist, "&/") {
			t.Fatalf("album should not be grouped under a combination artist: %+v", al)
		}
	}
	byAlbum := map[string]AlbumGroup{}
	for _, al := range albums {
		byAlbum[al.Album] = al
	}
	if g := byAlbum["Collision Course"]; g.Artist != "Linkin Park" || g.Tracks != 1 {
		t.Fatalf("Collision Course grouped as %+v", g)
	}
	if g := byAlbum["Fellini 2001 Tour"]; g.Artist != "БИ-2" || g.Tracks != 1 {
		t.Fatalf("Fellini 2001 Tour grouped as %+v", g)
	}
	if g := byAlbum["Ангел-Демон"]; g.Artist != "Король и Шут" {
		t.Fatalf("Ангел-Демон grouped as %+v", g)
	}
}
