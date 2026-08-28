package core

import "sync"

// Preferences holds the small set of per-user values PantryLens remembers
// across visits, purely to prefill the intake form -- it's deliberately
// separate from DietaryLens/LensStore (which the agent reads and writes
// conversationally through tools) since these are plain structured fields
// with no compliance logic of their own, set directly by the frontend
// rather than by the agent. Zero value for LastServings means "none saved
// yet"; LastCuisine's zero value ("") already means "no preference" the
// same way the intake form's optional cuisine field does.
type Preferences struct {
	LastServings int
	LastCuisine  string
}

// PreferenceStore is where a user's Preferences are kept. The default is
// an in-memory map (see NewPreferenceStore); see firestore_store.go for a
// Firestore-backed implementation, used so preferences survive process
// restarts (e.g. Cloud Run cold starts) once this moves past local
// testing.
type PreferenceStore interface {
	// Get returns false if userID has no stored preferences yet.
	Get(userID string) (Preferences, bool)
	Save(userID string, prefs Preferences)
}

type inMemoryPreferenceStore struct {
	mu     sync.RWMutex
	byUser map[string]Preferences
}

// NewPreferenceStore returns a PreferenceStore backed by an in-memory map.
func NewPreferenceStore() PreferenceStore {
	return &inMemoryPreferenceStore{byUser: make(map[string]Preferences)}
}

func (s *inMemoryPreferenceStore) Get(userID string) (Preferences, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	prefs, ok := s.byUser[userID]
	return prefs, ok
}

func (s *inMemoryPreferenceStore) Save(userID string, prefs Preferences) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byUser[userID] = prefs
}
