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

type firestoreLensStore struct {
	client *firestore.Client
}

// LensStore's interface has no error returns (the in-memory store can't
// fail), so a transient Firestore error here is logged and treated as "no
// data" rather than propagated -- callers fall back to whatever built-in
// presets they already have instead of crashing the conversation over a
// storage hiccup. Acceptable for a hackathon-scale feature; a production
// version would want SaveCustomLens/GetLensPreset to surface these.
func (s *firestoreLensStore) List() []DietaryLens {
	ctx := context.Background()
	it := s.client.Collection(lensCollection).Documents(ctx)
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

func (s *firestoreLensStore) Get(name string) (DietaryLens, bool) {
	ctx := context.Background()
	doc, err := s.client.Collection(lensCollection).Doc(name).Get(ctx)
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

func (s *firestoreLensStore) Save(lens DietaryLens) {
	ctx := context.Background()
	_, err := s.client.Collection(lensCollection).Doc(lens.Name).Set(ctx, toFirestoreDoc(lens))
	if err != nil {
		log.Printf("firestoreLensStore.Save %s: %v", lens.Name, err)
	}
}

// NewFirestoreRegistry returns a Registry whose custom lenses are stored in
// Firestore (collection "dietary_lenses", document ID = lens name) instead
// of an in-memory map.
func NewFirestoreRegistry(ctx context.Context, projectID string) (*Registry, error) {
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create firestore client: %w", err)
	}
	return &Registry{store: &firestoreLensStore{client: client}}, nil
}
