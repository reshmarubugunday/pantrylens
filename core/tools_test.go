package core

import "testing"

func contains(items []string, target string) bool {
	for _, it := range items {
		if it == target {
			return true
		}
	}
	return false
}

func TestListLensPresetsIncludesBothBuiltIns(t *testing.T) {
	r := NewRegistry()
	names := map[string]bool{}
	for _, s := range r.ListLensPresets() {
		names[s.Name] = true
	}
	if !names["GERD + Hormonal Balance + Macro Target"] {
		t.Error("expected GERD/hormonal/macro preset in list")
	}
	if !names["Athletic Performance"] {
		t.Error("expected Athletic Performance preset in list")
	}
}

func TestGetLensPresetUnknownNameReturnsError(t *testing.T) {
	r := NewRegistry()
	_, err := r.GetLensPreset("Not A Real Lens")
	if err == nil {
		t.Error("expected an error for an unknown lens name")
	}
}

func TestGetLensPresetReturnsFullDefinition(t *testing.T) {
	r := NewRegistry()
	lens, err := r.GetLensPreset("Athletic Performance")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(lens.PreferIngredients, "lean protein") {
		t.Error("expected 'lean protein' in prefer_ingredients")
	}
	if lens.MacroTargets.ProteinG == nil || *lens.MacroTargets.ProteinG != 45 {
		t.Error("expected protein target of 45")
	}
}

func TestComplianceCheckFlagsBannedIngredient(t *testing.T) {
	// "spicy chili" is a banned entry on this lens; "chili flakes" shares
	// the word "chili" so the token-overlap match should catch it. This is
	// a literal/keyword match, not food-category knowledge -- see the
	// "Known limitation" note above CheckRecipeAgainstLens's mentions() call
	// for why "orange juice" wouldn't be caught by a "citrus" avoid-entry
	// without a synonym table.
	r := NewRegistry()
	result, err := r.CheckRecipeAgainstLens(CheckRecipeInput{
		RecipeTitle: "Spicy Chicken",
		Ingredients: []string{"chicken breast", "chili flakes"},
		LensName:    "GERD + Hormonal Balance + Macro Target",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Compliant {
		t.Error("expected recipe to be non-compliant")
	}
	if !contains(result.Violations, "spicy chili") {
		t.Errorf("expected 'spicy chili' in violations, got %v", result.Violations)
	}
}

func TestComplianceCheckPassesCleanRecipe(t *testing.T) {
	r := NewRegistry()
	protein, carbs, fat, calories := 32, 38, 12, 450
	result, err := r.CheckRecipeAgainstLens(CheckRecipeInput{
		RecipeTitle: "Ginger Greens Bowl",
		Ingredients: []string{"spinach", "grilled chicken breast", "ginger", "brown rice"},
		LensName:    "GERD + Hormonal Balance + Macro Target",
		Calories:    &calories,
		ProteinG:    &protein,
		CarbsG:      &carbs,
		FatG:        &fat,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Compliant {
		t.Errorf("expected compliant recipe, got violations: %v", result.Violations)
	}
	if len(result.Violations) != 0 {
		t.Errorf("expected no violations, got %v", result.Violations)
	}
}

func TestComplianceCheckWarnsOnMacroDrift(t *testing.T) {
	r := NewRegistry()
	protein, carbs, fat, calories := 90, 70, 15, 650 // target protein is 45 -> should warn
	result, err := r.CheckRecipeAgainstLens(CheckRecipeInput{
		RecipeTitle: "Huge Protein Bowl",
		Ingredients: []string{"chicken breast", "quinoa", "spinach"},
		LensName:    "Athletic Performance",
		Calories:    &calories,
		ProteinG:    &protein,
		CarbsG:      &carbs,
		FatG:        &fat,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Compliant {
		t.Error("expected compliant (no banned ingredients), just with warnings")
	}
	found := false
	for _, w := range result.Warnings {
		if len(w) >= 7 && w[:7] == "Protein" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a protein drift warning, got %v", result.Warnings)
	}
}

func TestUnknownLensReturnsError(t *testing.T) {
	r := NewRegistry()
	_, err := r.CheckRecipeAgainstLens(CheckRecipeInput{
		RecipeTitle: "Mystery Dish",
		Ingredients: []string{"something"},
		LensName:    "Nonexistent Lens",
	})
	if err == nil {
		t.Error("expected an error for an unknown lens")
	}
}

func TestSaveAndReuseCustomLens(t *testing.T) {
	r := NewRegistry()
	lowCal := 500
	lowProtein, lowCarbs, lowFat := 30, 50, 15
	r.SaveCustomLens(DietaryLens{
		Name:              "Low FODMAP",
		AvoidIngredients:  []string{"garlic", "onion", "wheat"},
		PreferIngredients: []string{"rice", "carrot", "chicken"},
		CustomRules:       "Avoid high-FODMAP fruits and legumes.",
		MacroTargets: MacroTargets{
			Calories: &lowCal, ProteinG: &lowProtein, CarbsG: &lowCarbs, FatG: &lowFat,
		},
	})

	names := map[string]bool{}
	for _, s := range r.ListLensPresets() {
		names[s.Name] = true
	}
	if !names["Low FODMAP"] {
		t.Fatal("expected 'Low FODMAP' to be listed after saving")
	}

	result, err := r.CheckRecipeAgainstLens(CheckRecipeInput{
		RecipeTitle: "Garlic Onion Soup",
		Ingredients: []string{"garlic", "onion", "vegetable broth"},
		LensName:    "Low FODMAP",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Compliant {
		t.Error("expected non-compliant recipe")
	}
	if !contains(result.Violations, "garlic") || !contains(result.Violations, "onion") {
		t.Errorf("expected garlic and onion violations, got %v", result.Violations)
	}
}
