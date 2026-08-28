package core

// Firestore-backed LensStore, opt-in via NewFirestoreRegistry. This is the
// only file in core that imports anything beyond the Go standard library --
// everything else (the lens model, the compliance checker, the in-memory
// store) stays dependency-free and testable offline. Pull this in only
// when you want custom lenses to survive process restarts (e.g. Cloud Run
// cold starts); the in-memory store from NewRegistry is otherwise
// sufficient, since built-in presets always come from BuiltInLenses()
// regardless of storage backend.

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

const lensCollection = "dietary_lenses"

// firestoreLensDoc mirrors DietaryLens in a shape Firestore can
// (de)serialize directly -- a plain struct of flat fields, no pointers-to-
// pointers or embedded types that would need custom marshaling.
type firestoreLensDoc struct {
	Name              string
	AvoidIngredients  []string
	PreferIngredients []string
	CustomRules       string
	NotesStyle        string
	Calories          *int
	ProteinG          *int
	CarbsG            *int
	FatG              *int
}

func toFirestoreDoc(l DietaryLens) firestoreLensDoc {
	return firestoreLensDoc{
		Name:              l.Name,
		AvoidIngredients:  l.AvoidIngredients,
		PreferIngredients: l.PreferIngredients,
		CustomRules:       l.CustomRules,
		NotesStyle:        l.NotesStyle,
		Calories:          l.MacroTargets.Calories,
		ProteinG:          l.MacroTargets.ProteinG,
		CarbsG:            l.MacroTargets.CarbsG,
		FatG:              l.MacroTargets.FatG,
	}
}

func fromFirestoreDoc(d firestoreLensDoc) DietaryLens {
	return DietaryLens{
		Name:              d.Name,
		AvoidIngredients:  d.AvoidIngredients,
		PreferIngredients: d.PreferIngredients,
		CustomRules:       d.CustomRules,
		NotesStyle:        d.NotesStyle,
		MacroTargets: MacroTargets{
			Calories: d.Calories,
			ProteinG: d.ProteinG,
			CarbsG:   d.CarbsG,
			FatG:     d.FatG,
		},
	}
}

// usersCollection is the parent collection every per-user document lives
// under: "users/{userID}" holds a user's Preferences fields directly, and
// "users/{userID}/dietary_lenses/{lensName}" holds their custom lenses as a
// subcollection -- a document can carry both its own fields and
// subcollections in Firestore, so the two features share one parent doc
// per user without colliding.
const usersCollection = "users"

// sanitizeUserID guards against empty/malformed Firestore document IDs (an
// empty string, ".", "..", or one containing "/" all get rejected by the
// client) -- a caller with no real user ID still gets a working, if
// unscoped-from-any-other-anonymous-caller, document instead of every
// Firestore call failing outright.
func sanitizeUserID(userID string) string {
	if userID == "" || userID == "." || userID == ".." || strings.Contains(userID, "/") {
		return "_anonymous"
	}
	return userID
}

type firestoreLensStore struct {
	client *firestore.Client
}

func (s *firestoreLensStore) lensesCollection(userID string) *firestore.CollectionRef {
	return s.client.Collection(usersCollection).Doc(sanitizeUserID(userID)).Collection(lensCollection)
}

// LensStore's interface has no error returns (the in-memory store can't
// fail), so a transient Firestore error here is logged and treated as "no
// data" rather than propagated -- callers fall back to whatever built-in
// presets they already have instead of crashing the conversation over a
// storage hiccup. Acceptable for a hackathon-scale feature; a production
// version would want SaveCustomLens/GetLensPreset to surface these.
func (s *firestoreLensStore) List(userID string) []DietaryLens {
	ctx := context.Background()
	it := s.lensesCollection(userID).Documents(ctx)
	defer it.Stop()

	var lenses []DietaryLens
	for {
		doc, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("firestoreLensStore.List: %v", err)
			break
		}
		var d firestoreLensDoc
		if err := doc.DataTo(&d); err != nil {
			log.Printf("firestoreLensStore.List: decode %s: %v", doc.Ref.ID, err)
			continue
		}
		lenses = append(lenses, fromFirestoreDoc(d))
	}
	return lenses
}

func (s *firestoreLensStore) Get(userID, name string) (DietaryLens, bool) {
	ctx := context.Background()
	doc, err := s.lensesCollection(userID).Doc(name).Get(ctx)
	if err != nil {
		return DietaryLens{}, false
	}
	var d firestoreLensDoc
	if err := doc.DataTo(&d); err != nil {
		log.Printf("firestoreLensStore.Get: decode %s: %v", name, err)
		return DietaryLens{}, false
	}
	return fromFirestoreDoc(d), true
}

func (s *firestoreLensStore) Save(userID string, lens DietaryLens) {
	ctx := context.Background()
	_, err := s.lensesCollection(userID).Doc(lens.Name).Set(ctx, toFirestoreDoc(lens))
	if err != nil {
		log.Printf("firestoreLensStore.Save %s: %v", lens.Name, err)
	}
}

// NewFirestoreRegistry returns a Registry whose custom lenses are stored in
// Firestore, one document per lens under "users/{userID}/dietary_lenses",
// instead of an in-memory map.
func NewFirestoreRegistry(ctx context.Context, projectID string) (*Registry, error) {
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create firestore client: %w", err)
	}
	return &Registry{store: &firestoreLensStore{client: client}}, nil
}

// firestorePreferencesDoc mirrors Preferences in a shape Firestore can
// (de)serialize directly, stored as fields on the "users/{userID}" document
// itself (see usersCollection).
type firestorePreferencesDoc struct {
	LastServings int
	LastCuisine  string
}

type firestorePreferenceStore struct {
	client *firestore.Client
}

// Get returns false if the user has no stored preferences yet (a new user,
// or a transient Firestore error -- see the LensStore comment above on why
// errors are swallowed here rather than propagated).
func (s *firestorePreferenceStore) Get(userID string) (Preferences, bool) {
	ctx := context.Background()
	doc, err := s.client.Collection(usersCollection).Doc(sanitizeUserID(userID)).Get(ctx)
	if err != nil {
		return Preferences{}, false
	}
	var d firestorePreferencesDoc
	if err := doc.DataTo(&d); err != nil {
		log.Printf("firestorePreferenceStore.Get: decode %s: %v", userID, err)
		return Preferences{}, false
	}
	return Preferences{LastServings: d.LastServings, LastCuisine: d.LastCuisine}, true
}

func (s *firestorePreferenceStore) Save(userID string, prefs Preferences) {
	ctx := context.Background()
	// MergeAll rather than Set/overwrite: this document's ID doubles as the
	// parent of that user's "dietary_lenses" subcollection (see
	// usersCollection), so a partial write here should only ever touch
	// these two fields, never risk clobbering something else stored on it
	// later. The firestore client only accepts MergeAll with map data, not
	// a struct, hence building one by hand here instead of reusing
	// firestorePreferencesDoc directly.
	doc := map[string]any{"LastServings": prefs.LastServings, "LastCuisine": prefs.LastCuisine}
	_, err := s.client.Collection(usersCollection).Doc(sanitizeUserID(userID)).Set(ctx, doc, firestore.MergeAll)
	if err != nil {
		log.Printf("firestorePreferenceStore.Save %s: %v", userID, err)
	}
}

// NewFirestorePreferenceStore returns a PreferenceStore backed by Firestore
// (see usersCollection) instead of an in-memory map.
func NewFirestorePreferenceStore(ctx context.Context, projectID string) (PreferenceStore, error) {
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create firestore client: %w", err)
	}
	return &firestorePreferenceStore{client: client}, nil
}

// savedRecipesCollection holds one document per saved recipe, keyed by the
// random ID minted when it was saved (see ../app/cmd/pantrylens/frontend
// /frontend.go) -- a flat, unscoped collection, since a saved recipe's
// "view" link is meant to work like a paste/share link (anyone with the
// unguessable ID can open it), not something tied to a signed-in user.
// savedRecipeRefsCollection, by contrast, is scoped under usersCollection
// (see the const above) -- it's the index "My saved recipes" reads from,
// so a listing only ever shows what that user saved without needing to
// fetch every full recipe just to list them.
const (
	savedRecipesCollection    = "saved_recipes"
	savedRecipeRefsCollection = "saved_recipe_refs"
)

// firestoreRecipeDoc mirrors SavedRecipe in a shape Firestore can
// (de)serialize directly.
type firestoreRecipeDoc struct {
	Title                 string
	Cuisine               string
	Servings              int
	Ingredients           []string
	Steps                 []string
	Calories              *int
	ProteinG              *int
	CarbsG                *int
	FatG                  *int
	LensNote              string
	StorageNote           string
	AdditionalIngredients []string
	MealPrepBatch         string
	MealType              string
}

func toFirestoreRecipeDoc(r SavedRecipe) firestoreRecipeDoc {
	return firestoreRecipeDoc{
		Title:                 r.Title,
		Cuisine:               r.Cuisine,
		Servings:              r.Servings,
		Ingredients:           r.Ingredients,
		Steps:                 r.Steps,
		Calories:              r.Calories,
		ProteinG:              r.ProteinG,
		CarbsG:                r.CarbsG,
		FatG:                  r.FatG,
		LensNote:              r.LensNote,
		StorageNote:           r.StorageNote,
		AdditionalIngredients: r.AdditionalIngredients,
		MealPrepBatch:         r.MealPrepBatch,
		MealType:              r.MealType,
	}
}

func fromFirestoreRecipeDoc(d firestoreRecipeDoc) SavedRecipe {
	return SavedRecipe{
		Title:                 d.Title,
		Cuisine:               d.Cuisine,
		Servings:              d.Servings,
		Ingredients:           d.Ingredients,
		Steps:                 d.Steps,
		Calories:              d.Calories,
		ProteinG:              d.ProteinG,
		CarbsG:                d.CarbsG,
		FatG:                  d.FatG,
		LensNote:              d.LensNote,
		StorageNote:           d.StorageNote,
		AdditionalIngredients: d.AdditionalIngredients,
		MealPrepBatch:         d.MealPrepBatch,
		MealType:              d.MealType,
	}
}

// firestoreRecipeRefDoc is the per-user index entry List reads from --
// just enough to render "My saved recipes" without fetching every full
// recipe.
type firestoreRecipeRefDoc struct {
	Title         string
	SavedAt       time.Time
	MealPrepBatch string
	MealType      string
}

type firestoreRecipeStore struct {
	client *firestore.Client
}

// Save's error IS propagated (unlike LensStore/PreferenceStore's
// swallow-and-log approach) -- see the RecipeStore doc comment on why a
// saved-recipe link can't fail silently. It writes two documents: the full
// recipe (flat, unscoped, for Get) and a lightweight per-user index entry
// (for List) -- not atomic across both, but a partial failure here just
// means the recipe doesn't show up in "My saved recipes" while its direct
// link still works, not the reverse (data loss), so the tradeoff favors
// keeping this simple over wrapping it in a transaction.
func (s *firestoreRecipeStore) Save(userID, id string, recipe SavedRecipe) error {
	ctx := context.Background()
	if _, err := s.client.Collection(savedRecipesCollection).Doc(id).Set(ctx, toFirestoreRecipeDoc(recipe)); err != nil {
		return fmt.Errorf("save recipe %s: %w", id, err)
	}
	ref := s.client.Collection(usersCollection).Doc(sanitizeUserID(userID)).Collection(savedRecipeRefsCollection).Doc(id)
	refDoc := firestoreRecipeRefDoc{
		Title: recipe.Title, SavedAt: time.Now(),
		MealPrepBatch: recipe.MealPrepBatch, MealType: recipe.MealType,
	}
	if _, err := ref.Set(ctx, refDoc); err != nil {
		return fmt.Errorf("index recipe %s for user: %w", id, err)
	}
	return nil
}

func (s *firestoreRecipeStore) List(userID string) []SavedRecipeSummary {
	ctx := context.Background()
	it := s.client.Collection(usersCollection).Doc(sanitizeUserID(userID)).Collection(savedRecipeRefsCollection).
		OrderBy("SavedAt", firestore.Desc).Documents(ctx)
	defer it.Stop()

	var summaries []SavedRecipeSummary
	for {
		doc, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("firestoreRecipeStore.List: %v", err)
			break
		}
		var d firestoreRecipeRefDoc
		if err := doc.DataTo(&d); err != nil {
			log.Printf("firestoreRecipeStore.List: decode %s: %v", doc.Ref.ID, err)
			continue
		}
		summaries = append(summaries, SavedRecipeSummary{
			ID: doc.Ref.ID, Title: d.Title, SavedAt: d.SavedAt,
			MealPrepBatch: d.MealPrepBatch, MealType: d.MealType,
		})
	}
	return summaries
}

func (s *firestoreRecipeStore) Get(id string) (SavedRecipe, bool) {
	ctx := context.Background()
	doc, err := s.client.Collection(savedRecipesCollection).Doc(id).Get(ctx)
	if err != nil {
		return SavedRecipe{}, false
	}
	var d firestoreRecipeDoc
	if err := doc.DataTo(&d); err != nil {
		log.Printf("firestoreRecipeStore.Get: decode %s: %v", id, err)
		return SavedRecipe{}, false
	}
	return fromFirestoreRecipeDoc(d), true
}

// NewFirestoreRecipeStore returns a RecipeStore backed by Firestore (see
// savedRecipesCollection) instead of an in-memory map.
func NewFirestoreRecipeStore(ctx context.Context, projectID string) (RecipeStore, error) {
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create firestore client: %w", err)
	}
	return &firestoreRecipeStore{client: client}, nil
}
