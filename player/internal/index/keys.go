package index

import (
	"strconv"
	"time"

	"github.com/torwin-job/musik/player/internal/db"
)

// ArtistKey normalizes an artist name for rules and entity vectors.
func ArtistKey(artist string) string {
	return normName(artist)
}

// AlbumKey builds artist|album for album-level identity.
func AlbumKey(artist, album string) string {
	a, al := normName(artist), normName(album)
	if al == "" {
		return ""
	}
	if a == "" {
		return al
	}
	return a + "|" + al
}

// TrackKey is the stable radio-rule key for a catalog track.
func TrackKey(id int64) string {
	return strconv.FormatInt(id, 10)
}

// ClusterKey is the radio-rule key for a cluster id.
func ClusterKey(id int) string {
	return strconv.Itoa(id)
}

// RobustArtistVector averages album centroids and drops atypical albums so one
// outlying record does not pull the artist embedding.
func RobustArtistVector(albumVectors [][]float32) []float32 {
	if len(albumVectors) == 0 {
		return nil
	}
	dim := 0
	for _, vector := range albumVectors {
		if len(vector) > dim {
			dim = len(vector)
		}
	}
	if dim == 0 {
		return nil
	}
	mean := meanOf(albumVectors, dim)
	if len(albumVectors) < 3 {
		return mean
	}
	kept := make([][]float32, 0, len(albumVectors))
	for _, vector := range albumVectors {
		if len(vector) != dim {
			continue
		}
		if cosine(mean, vector) >= 0.25 {
			kept = append(kept, vector)
		}
	}
	if len(kept) == 0 {
		return mean
	}
	return meanOf(kept, dim)
}

func meanOf(vectors [][]float32, dim int) []float32 {
	out := make([]float32, dim)
	var n int
	for _, vector := range vectors {
		if len(vector) != dim {
			continue
		}
		for i := range out {
			out[i] += vector[i]
		}
		n++
	}
	if n == 0 {
		return nil
	}
	inv := 1 / float32(n)
	for i := range out {
		out[i] *= inv
	}
	Normalize(out)
	return out
}

func cosine(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

func (idx *Index) EntityVectorRows(model string) []db.EntityVector {
	if model == "" {
		model = db.EntityModelVersion
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	albums := idx.AlbumCentroids()
	artists := idx.ArtistCentroids()
	byArtist := map[string][][]float32{}
	out := make([]db.EntityVector, 0, len(albums)+len(artists))
	for _, album := range albums {
		key := AlbumKey(album.Artist, album.Album)
		if key == "" || len(album.Vector) == 0 {
			continue
		}
		out = append(out, db.EntityVector{
			Type: "album", Key: key, Embedding: Float32Bytes(album.Vector),
			Dim: len(album.Vector), Model: model, TrackCount: len(album.Rows), ComputedAt: now,
		})
		byArtist[ArtistKey(album.Artist)] = append(byArtist[ArtistKey(album.Artist)], album.Vector)
	}
	for _, artist := range artists {
		key := ArtistKey(artist.Artist)
		if key == "" {
			continue
		}
		vec := RobustArtistVector(byArtist[key])
		if len(vec) == 0 {
			vec = artist.Vector
		}
		if len(vec) == 0 {
			continue
		}
		out = append(out, db.EntityVector{
			Type: "artist", Key: key, Embedding: Float32Bytes(vec),
			Dim: len(vec), Model: model, TrackCount: len(artist.Rows), ComputedAt: now,
		})
	}
	return out
}
