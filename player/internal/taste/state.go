package taste

import (
	"math"
	"sync"
	"time"

	"github.com/torwin-job/musik/player/internal/index"
)

const (
	DefaultDaypartMinimum = 5
	maxNegativePrototypes = 8
	maxRecentPositive     = 8
)

type VectorState struct {
	Vector  []float32 `json:"vector,omitempty"`
	Samples int       `json:"samples"`
}

type NegativePrototype struct {
	Vector    []float32 `json:"vector"`
	Mass      float64   `json:"mass"`
	Samples   int       `json:"samples"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SessionState struct {
	Positive       VectorState         `json:"positive"`
	Negative       []NegativePrototype `json:"negative,omitempty"`
	RecentPositive [][]float32         `json:"recent_positive,omitempty"`
}

type State struct {
	mu         sync.RWMutex
	Long       *Profile
	Dayparts   map[string]VectorState
	Persistent []NegativePrototype
}

func NewState(long *Profile) *State {
	if long == nil {
		long = New()
	}
	return &State{Long: long, Dayparts: map[string]VectorState{}}
}

func ApplyPositive(state VectorState, vector []float32, weight, alpha float64) VectorState {
	return updatePositive(state, vector, weight, alpha)
}

func updatePositive(state VectorState, vector []float32, weight, alpha float64) VectorState {
	if len(vector) == 0 || weight <= 0 {
		return state
	}
	if alpha <= 0 || alpha > 1 {
		alpha = 0.1
	}
	if len(state.Vector) != len(vector) {
		state.Vector = append([]float32(nil), vector...)
	} else {
		a := float32(alpha)
		w := float32(math.Min(weight, 2))
		for i := range state.Vector {
			state.Vector[i] = (1-a)*state.Vector[i] + a*w*vector[i]
		}
	}
	index.Normalize(state.Vector)
	state.Samples++
	return state
}

func (s *State) UpdatePositive(
	vector []float32,
	weight, alpha float64,
	daypart string,
	session *SessionState,
) {
	if len(vector) == 0 || weight <= 0 {
		return
	}
	s.Long.UpdateEMA(vector, weight, alpha)
	s.mu.Lock()
	s.Dayparts[daypart] = updatePositive(s.Dayparts[daypart], vector, weight, alpha)
	s.mu.Unlock()
	if session != nil {
		session.Positive = updatePositive(session.Positive, vector, weight, alpha)
		recent := append([]float32(nil), vector...)
		index.Normalize(recent)
		session.RecentPositive = append(session.RecentPositive, recent)
		if len(session.RecentPositive) > maxRecentPositive {
			session.RecentPositive = append([][]float32(nil),
				session.RecentPositive[len(session.RecentPositive)-maxRecentPositive:]...)
		}
	}
}

func addNegative(prototypes []NegativePrototype, vector []float32, weight float64) []NegativePrototype {
	if len(vector) == 0 || weight <= 0 {
		return prototypes
	}
	v := append([]float32(nil), vector...)
	index.Normalize(v)
	best, bestSimilarity := -1, float32(-1)
	for i := range prototypes {
		sim := dot(prototypes[i].Vector, v)
		if sim > bestSimilarity {
			best, bestSimilarity = i, sim
		}
	}
	now := time.Now().UTC()
	if best >= 0 && bestSimilarity >= 0.82 {
		p := &prototypes[best]
		total := p.Mass + weight
		for i := range p.Vector {
			p.Vector[i] = float32(
				(float64(p.Vector[i])*p.Mass + float64(v[i])*weight) / total,
			)
		}
		index.Normalize(p.Vector)
		p.Mass = total
		p.Samples++
		p.UpdatedAt = now
		return prototypes
	}
	prototypes = append(prototypes, NegativePrototype{
		Vector: v, Mass: weight, Samples: 1, UpdatedAt: now,
	})
	if len(prototypes) <= maxNegativePrototypes {
		return prototypes
	}
	lightest := 0
	for i := 1; i < len(prototypes); i++ {
		if prototypes[i].Mass < prototypes[lightest].Mass {
			lightest = i
		}
	}
	return append(prototypes[:lightest], prototypes[lightest+1:]...)
}

// AddEarlySkip affects only the current listening session.
func (s *State) AddEarlySkip(session *SessionState, vector []float32, weight float64) {
	if session == nil {
		return
	}
	session.Negative = addNegative(session.Negative, vector, weight)
}

// AddDislike records a persistent negative prototype without subtracting the
// track from the positive long/daypart/session centroids.
func (s *State) AddDislike(vector []float32, weight float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Persistent = addNegative(s.Persistent, vector, weight)
	s.Long.mu.Lock()
	s.Long.NNegative++
	s.Long.mu.Unlock()
}

func (s *State) Effective(daypart string, session *SessionState, daypartMinimum int) []float32 {
	if daypartMinimum < 1 {
		daypartMinimum = DefaultDaypartMinimum
	}
	long := s.Long.Get()
	s.mu.RLock()
	dp := s.Dayparts[daypart]
	s.mu.RUnlock()
	if len(long) == 0 {
		if session != nil {
			return normalizedCopy(session.Positive.Vector)
		}
		return nil
	}
	out := make([]float32, len(long))
	copy(out, long)
	total := float32(1)
	if dp.Samples >= daypartMinimum && len(dp.Vector) == len(out) {
		for i := range out {
			out[i] += 0.4 * dp.Vector[i]
		}
		total += 0.4
	}
	if session != nil && len(session.Positive.Vector) == len(out) {
		for i := range out {
			out[i] += 0.8 * session.Positive.Vector[i]
		}
		total += 0.8
	}
	// Recent completed tracks provide a small coherence term without becoming a
	// fourth persistent taste vector.
	if session != nil && len(session.RecentPositive) > 0 {
		recent := meanDirection(session.RecentPositive, len(out))
		if len(recent) == len(out) {
			for i := range out {
				out[i] += 0.25 * recent[i]
			}
			total += 0.25
		}
	}
	for i := range out {
		out[i] /= total
	}
	index.Normalize(out)
	return out
}

func (s *State) NegativePenalty(vector []float32, session *SessionState) float64 {
	s.mu.RLock()
	persistent := append([]NegativePrototype(nil), s.Persistent...)
	s.mu.RUnlock()
	all := persistent
	if session != nil {
		all = append(all, session.Negative...)
	}
	var penalty float64
	for _, prototype := range all {
		sim := float64(dot(prototype.Vector, vector))
		if sim <= 0 {
			continue
		}
		value := sim * math.Min(1, 0.25+math.Log1p(prototype.Mass)/2)
		if value > penalty {
			penalty = value
		}
	}
	return penalty
}

func (s *State) Daypart(daypart string) VectorState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state := s.Dayparts[daypart]
	state.Vector = append([]float32(nil), state.Vector...)
	return state
}

func (s *State) RestoreDaypart(daypart string, state VectorState) {
	state.Vector = normalizedCopy(state.Vector)
	s.mu.Lock()
	s.Dayparts[daypart] = state
	s.mu.Unlock()
}

func (s *State) RestorePersistentNegatives(prototypes []NegativePrototype) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Persistent = append([]NegativePrototype(nil), prototypes...)
	for i := range s.Persistent {
		s.Persistent[i].Vector = normalizedCopy(s.Persistent[i].Vector)
	}
}

func (s *State) PersistentNegatives() []NegativePrototype {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]NegativePrototype, len(s.Persistent))
	copy(out, s.Persistent)
	for i := range out {
		out[i].Vector = append([]float32(nil), out[i].Vector...)
	}
	return out
}

func normalizedCopy(vector []float32) []float32 {
	if len(vector) == 0 {
		return nil
	}
	out := append([]float32(nil), vector...)
	index.Normalize(out)
	return out
}

func meanDirection(vectors [][]float32, dim int) []float32 {
	if dim == 0 {
		return nil
	}
	out := make([]float32, dim)
	var count int
	for _, vector := range vectors {
		if len(vector) != dim {
			continue
		}
		for i := range out {
			out[i] += vector[i]
		}
		count++
	}
	if count == 0 {
		return nil
	}
	index.Normalize(out)
	return out
}

func dot(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}
