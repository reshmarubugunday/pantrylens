package core

import "testing"

func TestRecipeStoreSaveAndGet(t *testing.T) {
	s := NewRecipeStore()
	if err := s.Save("user-1", "recipe-1", SavedRecipe{Title: "Ginger Greens Bowl"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	recipe, ok := s.Get("recipe-1")
	if !ok {
		t.Fatal("expected recipe to be found")
	}
	if recipe.Title != "Ginger Greens Bowl" {
		t.Errorf("got title %q", recipe.Title)
	}
	if _, ok := s.Get("no-such-id"); ok {
		t.Error("expected no recipe for an unknown ID")
	}
}

func TestRecipeStoreGetIsNotScopedToUser(t *testing.T) {
	// A recipe's view link works for anyone who has it, regardless of
	// which user saved it -- see the RecipeStore doc comment.
	s := NewRecipeStore()
	if err := s.Save("user-1", "recipe-1", SavedRecipe{Title: "Ginger Greens Bowl"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := s.Get("recipe-1"); !ok {
		t.Error("expected the recipe to be gettable without needing user-1's ID")
	}
}

func TestRecipeStoreListScopedPerUserMostRecentFirst(t *testing.T) {
	s := NewRecipeStore()
	if err := s.Save("user-1", "recipe-1", SavedRecipe{Title: "First"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := s.Save("user-1", "recipe-2", SavedRecipe{Title: "Second"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := s.Save("user-2", "recipe-3", SavedRecipe{Title: "Someone else's"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	list := s.List("user-1")
	if len(list) != 2 {
		t.Fatalf("expected 2 saved recipes for user-1, got %d", len(list))
	}
	if list[0].Title != "Second" || list[1].Title != "First" {
		t.Errorf("expected most-recently-saved first, got %v", list)
	}

	if list := s.List("user-2"); len(list) != 1 || list[0].Title != "Someone else's" {
		t.Errorf("expected user-2 to see only their own recipe, got %v", list)
	}

	if list := s.List("user-3"); len(list) != 0 {
		t.Errorf("expected no recipes for a user who hasn't saved any, got %v", list)
	}
}

func TestRecipeStoreCarriesMealPrepBatchThrough(t *testing.T) {
	s := NewRecipeStore()
	batch := "Meal prep -- Aug 27, 2026"
	if err := s.Save("user-1", "recipe-1", SavedRecipe{Title: "Bowl One", MealPrepBatch: batch}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := s.Save("user-1", "recipe-2", SavedRecipe{Title: "Solo Dinner"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	list := s.List("user-1")
	if len(list) != 2 {
		t.Fatalf("expected 2 saved recipes, got %d", len(list))
	}
	byTitle := map[string]SavedRecipeSummary{}
	for _, s := range list {
		byTitle[s.Title] = s
	}
	if byTitle["Bowl One"].MealPrepBatch != batch {
		t.Errorf("expected MealPrepBatch %q on the batch recipe's summary, got %q", batch, byTitle["Bowl One"].MealPrepBatch)
	}
	if byTitle["Solo Dinner"].MealPrepBatch != "" {
		t.Errorf("expected no MealPrepBatch on a solo recipe's summary, got %q", byTitle["Solo Dinner"].MealPrepBatch)
	}

	recipe, ok := s.Get("recipe-1")
	if !ok || recipe.MealPrepBatch != batch {
		t.Errorf("expected MealPrepBatch %q on the full saved recipe, got %q (ok=%v)", batch, recipe.MealPrepBatch, ok)
	}
}

func TestRecipeStoreCarriesMealTypeThrough(t *testing.T) {
	s := NewRecipeStore()
	if err := s.Save("user-1", "recipe-1", SavedRecipe{Title: "Omelette", MealType: "Breakfast"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := s.Save("user-1", "recipe-2", SavedRecipe{Title: "Untyped Bowl"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	list := s.List("user-1")
	byTitle := map[string]SavedRecipeSummary{}
	for _, s := range list {
		byTitle[s.Title] = s
	}
	if byTitle["Omelette"].MealType != "Breakfast" {
		t.Errorf("expected MealType %q on the summary, got %q", "Breakfast", byTitle["Omelette"].MealType)
	}
	if byTitle["Untyped Bowl"].MealType != "" {
		t.Errorf("expected no MealType on a recipe that didn't set one, got %q", byTitle["Untyped Bowl"].MealType)
	}

	recipe, ok := s.Get("recipe-1")
	if !ok || recipe.MealType != "Breakfast" {
		t.Errorf("expected MealType %q on the full saved recipe, got %q (ok=%v)", "Breakfast", recipe.MealType, ok)
	}
}
