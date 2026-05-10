package graphql

import (
	"sync"
	"time"
)

// ComplexityInfo captures monday.com complexity budget information from a
// GraphQL response. Fields are zero when the server did not provide the data.
type ComplexityInfo struct {
	// Used is the complexity units consumed by the last query.
	Used int
	// ResetAt is the time at which the complexity budget resets.
	// Zero if not known.
	ResetAt time.Time
}

// complexityStore is a mutex-guarded holder for the most recent ComplexityInfo.
type complexityStore struct {
	mu   sync.Mutex
	last ComplexityInfo
}

func (s *complexityStore) set(ci ComplexityInfo) {
	s.mu.Lock()
	s.last = ci
	s.mu.Unlock()
}

func (s *complexityStore) get() ComplexityInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}
