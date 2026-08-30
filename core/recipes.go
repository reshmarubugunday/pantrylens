package core

import (
	"sync"
	"time"
)

// SavedRecipe is a snapshot of one recipe card exactly as the user saw it
// (see propose_recipe in ../app/tools_adk.go), kept so it can be viewed
// again later -- it's a read-only copy for viewing, not something the
// agent reads back or reasons about.
type SavedRecipe struct {
	Title       string
	Cuisine     string
	Servings    int
	Ingredients []string
	Steps       []string
	Calories    *int
	ProteinG    *int
	CarbsG      *int
	FatG        *int
	LensNote    string
	StorageNote string
	// AdditionalIngredients is the subset of Ingredients (verbatim matches)
	// the user didn't already have -- see propose_recipe's field of the
	// same name in ../app/tools_adk.go.
	AdditionalIngredients []string
	// MealPrepBatch is a shared, human-readable label (e.g. "Meal prep --
	// Aug 27, 2026") for every recipe saved together from the same
	// meal-prep response (see static/app.js's "Save all" button) -- empty
	// for an ordinary single-meal recipe. It's what "My saved recipes"
	// groups by, purely a display label with no other structure to it (not
	// an ID, not a foreign key to some separate "meal plan" entity) -- kept
	// deliberately this lightweight rather than inventing a whole new
	// grouping concept for what's still just a list of SavedRecipes.
	MealPrepBatch string
	// MealType is one of "Breakfast", "Lunch", "Dinner", "Snack", or empty
	// if neither the user nor the model settled on one -- see
	// propose_recipe's field of the same name in ../app/tools_adk.go. It's
	// what "My saved recipes" filters by (see the tab bar in static
	// /index.html); a recipe with no MealType only shows up under "All".
	MealType string
}

// SavedRecipeSummary is one entry in a user's saved-recipes list -- enough
// to render "My saved recipes" (see ../app/cmd/pantrylens/frontend
// /frontend.go) without fetching every full SavedRecipe just to list them.
type SavedRecipeSummary struct {
	ID            string
	Title         string
	SavedAt       time.Time
	MealPrepBatch string
	MealType      string
}

// RecipeStore is where saved recipes are kept, scoped per user (the same
// anonymous, device-local ID already used for PreferenceStore -- there's
// no login in this app) so "My saved recipes" only ever shows what that
// browser saved. A recipe's own "view" page is still reachable by anyone
// with its link regardless of user, the same way a saved Google Doc link
// works -- Get takes just the ID, not a userID, on purpose. Unlike
// LensStore/PreferenceStore, a failed Save must be reported rather than
// swallowed -- the caller is about to hand the user a link, and a link to
// nothing silently succeeding would be worse than an error. The default is
// an in-memory map (see NewRecipeStore); see firestore_store.go for a
// Firestore-backed implementation, used so saved recipes survive process
// restarts (e.g. Cloud Run cold starts).
type RecipeStore interface {
	Save(userID, id string, recipe SavedRecipe) error
	// Get returns false if id doesn't match any saved recipe.
	Get(id string) (SavedRecipe, bool)
	// List returns userID's saved recipes, most recently saved first.
	List(userID string) []SavedRecipeSummary
	// Delete removes id from userID's saved recipes. It's a no-op (not an
	// error) if id doesn't exist or wasn't saved by userID -- deleting
	// something already gone, or someone else's recipe by guessing its ID,
	// both end in the same "not there" state either way.
	Delete(userID, id string) error
}

type inMemoryRecipeStore struct {
	mu      sync.RWMutex
	recipes map[string]SavedRecipe          // id -> recipe, for Get (unscoped)
	byUser  map[string][]SavedRecipeSummary // userID -> summaries, newest first
}

// NewRecipeStore returns a RecipeStore backed by an in-memory map.
func NewRecipeStore() RecipeStore {
	return &inMemoryRecipeStore{
		recipes: make(map[string]SavedRecipe),
		byUser:  make(map[string][]SavedRecipeSummary),
	}
}

func (s *inMemoryRecipeStore) Save(userID, id string, recipe SavedRecipe) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recipes[id] = recipe
	summary := SavedRecipeSummary{
		ID: id, Title: recipe.Title, SavedAt: time.Now(),
		MealPrepBatch: recipe.MealPrepBatch, MealType: recipe.MealType,
	}
	s.byUser[userID] = append([]SavedRecipeSummary{summary}, s.byUser[userID]...)
	return nil
}

func (s *inMemoryRecipeStore) Get(id string) (SavedRecipe, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	recipe, ok := s.recipes[id]
	return recipe, ok
}

func (s *inMemoryRecipeStore) List(userID string) []SavedRecipeSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]SavedRecipeSummary(nil), s.byUser[userID]...)
}

func (s *inMemoryRecipeStore) Delete(userID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	summaries := s.byUser[userID]
	for i, sum := range summaries {
		if sum.ID == id {
			s.byUser[userID] = append(summaries[:i], summaries[i+1:]...)
			delete(s.recipes, id)
			return nil
		}
	}
	return nil
}
