package taste

import (
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/torwin-job/musik/player/internal/index"
)

const CentroidAlgorithmVersion = "weighted-spherical-kmeans-v1"

type Sample struct {
	TrackID int64
	Vector  []float32
	Weight  float64
	At      time.Time
}

type Centroid struct {
	Vector      []float32
	Mass        float64
	SampleCount int
}

type CentroidOptions struct {
	MaxK                 int
	MinUniquePerCentroid int
	Iterations           int
	RecencyHalfLife      time.Duration
	PerTrackWeightCap    float64
	MergeSimilarity      float64
	Seed                 int64
	WarmStart            []Centroid
	Now                  time.Time
}

func defaultCentroidOptions(options CentroidOptions) CentroidOptions {
	if options.MaxK < 1 || options.MaxK > 6 {
		options.MaxK = 6
	}
	if options.MinUniquePerCentroid < 2 {
		options.MinUniquePerCentroid = 12
	}
	if options.Iterations < 1 {
		options.Iterations = 20
	}
	if options.RecencyHalfLife <= 0 {
		options.RecencyHalfLife = 90 * 24 * time.Hour
	}
	if options.PerTrackWeightCap <= 0 {
		options.PerTrackWeightCap = 3
	}
	if options.MergeSimilarity <= 0 {
		options.MergeSimilarity = 0.97
	}
	if options.Now.IsZero() {
		options.Now = time.Now().UTC()
	}
	return options
}

// WeightedSphericalCentroids performs deterministic weighted spherical
// k-means++. Repeated observations of one track are capped before clustering;
// recency affects mass but every centroid is normalized after each update.
func WeightedSphericalCentroids(samples []Sample, options CentroidOptions) []Centroid {
	options = defaultCentroidOptions(options)
	type point struct {
		vector  []float32
		weight  float64
		trackID int64
	}
	var points []point
	trackMass := map[int64]float64{}
	unique := map[int64]struct{}{}
	dim := 0
	for _, sample := range samples {
		if len(sample.Vector) == 0 || sample.Weight <= 0 {
			continue
		}
		if dim == 0 {
			dim = len(sample.Vector)
		}
		if len(sample.Vector) != dim {
			continue
		}
		weight := sample.Weight
		if !sample.At.IsZero() {
			age := options.Now.Sub(sample.At)
			if age > 0 {
				weight *= math.Pow(0.5, age.Hours()/options.RecencyHalfLife.Hours())
			}
		}
		remaining := options.PerTrackWeightCap - trackMass[sample.TrackID]
		if remaining <= 0 {
			continue
		}
		if weight > remaining {
			weight = remaining
		}
		if weight <= 0 {
			continue
		}
		vector := normalizedCopy(sample.Vector)
		points = append(points, point{vector: vector, weight: weight, trackID: sample.TrackID})
		trackMass[sample.TrackID] += weight
		unique[sample.TrackID] = struct{}{}
	}
	if len(points) == 0 {
		return nil
	}
	k := 1
	if len(unique) >= options.MinUniquePerCentroid*2 {
		k = len(unique) / options.MinUniquePerCentroid
		if k > options.MaxK {
			k = options.MaxK
		}
	}
	if k > len(points) {
		k = len(points)
	}

	centers := make([][]float32, 0, k)
	for _, warm := range options.WarmStart {
		if len(centers) == k {
			break
		}
		if len(warm.Vector) == dim {
			centers = append(centers, normalizedCopy(warm.Vector))
		}
	}
	rng := rand.New(rand.NewSource(options.Seed))
	if len(centers) == 0 {
		var total float64
		for _, point := range points {
			total += point.weight
		}
		target := rng.Float64() * total
		chosen := len(points) - 1
		for i, point := range points {
			target -= point.weight
			if target <= 0 {
				chosen = i
				break
			}
		}
		centers = append(centers, points[chosen].vector)
	}
	for len(centers) < k {
		distances := make([]float64, len(points))
		var total float64
		for i, point := range points {
			best := float64(-1)
			for _, center := range centers {
				if similarity := float64(dot(point.vector, center)); similarity > best {
					best = similarity
				}
			}
			distance := math.Max(0, 1-best)
			distances[i] = point.weight * distance * distance
			total += distances[i]
		}
		if total <= 1e-12 {
			break
		}
		target := rng.Float64() * total
		chosen := len(points) - 1
		for i, distance := range distances {
			target -= distance
			if target <= 0 {
				chosen = i
				break
			}
		}
		centers = append(centers, append([]float32(nil), points[chosen].vector...))
	}

	assignments := make([]int, len(points))
	for iteration := 0; iteration < options.Iterations; iteration++ {
		sums := make([][]float32, len(centers))
		masses := make([]float64, len(centers))
		for i := range sums {
			sums[i] = make([]float32, dim)
		}
		changed := false
		for i, point := range points {
			best, bestSimilarity := 0, float32(-2)
			for j, center := range centers {
				if similarity := dot(point.vector, center); similarity > bestSimilarity {
					best, bestSimilarity = j, similarity
				}
			}
			if assignments[i] != best || iteration == 0 {
				changed = true
			}
			assignments[i] = best
			masses[best] += point.weight
			for d := range sums[best] {
				sums[best][d] += float32(point.weight) * point.vector[d]
			}
		}
		for i := range centers {
			if masses[i] > 0 {
				index.Normalize(sums[i])
				centers[i] = sums[i]
			}
		}
		if !changed {
			break
		}
	}

	result := make([]Centroid, len(centers))
	for i, center := range centers {
		result[i].Vector = center
	}
	for i, point := range points {
		cluster := assignments[i]
		if cluster >= len(result) {
			continue
		}
		result[cluster].Mass += point.weight
		result[cluster].SampleCount++
	}
	result = mergeCentroids(result, options.MergeSimilarity)
	sort.Slice(result, func(i, j int) bool { return result[i].Mass > result[j].Mass })
	return result
}

func mergeCentroids(centroids []Centroid, threshold float64) []Centroid {
	var merged []Centroid
	for _, centroid := range centroids {
		if centroid.Mass <= 0 {
			continue
		}
		best := -1
		for i := range merged {
			if float64(dot(centroid.Vector, merged[i].Vector)) >= threshold {
				best = i
				break
			}
		}
		if best < 0 {
			merged = append(merged, centroid)
			continue
		}
		target := &merged[best]
		total := target.Mass + centroid.Mass
		for i := range target.Vector {
			target.Vector[i] = float32(
				(float64(target.Vector[i])*target.Mass +
					float64(centroid.Vector[i])*centroid.Mass) / total,
			)
		}
		index.Normalize(target.Vector)
		target.Mass = total
		target.SampleCount += centroid.SampleCount
	}
	return merged
}
