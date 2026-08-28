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

func TestListLensPresetsIncludesAllBuiltIns(t *testing.T) {
	r := NewRegistry()
	names := map[string]bool{}
	for _, s := range r.ListLensPresets("user-1") {
		names[s.Name] = true
	}
	for _, want := range []string{
		"GERD-Friendly",
		"Hormonal Balance",
		"Macro Target (30g Protein / 40g Carbs / 15g Fat)",
		"Athletic Performance",
		"Vegetarian",
		"Vegan",
		"Diabetic-Friendly",
		"Heart-Healthy (Low-Sodium)",
		"Gluten-Free",
		"Dairy-Free",
		"Keto (Low-Carb)",
		"Low-FODMAP",
		"Kidney-Friendly (Renal)",
	} {
		if !names[want] {
			t.Errorf("expected %q preset in list", want)
		}
	}
}

func TestKetoLensFlagsHighCarbIngredients(t *testing.T) {
	r := NewRegistry()
	result, err := r.CheckRecipeAgainstLens(CheckRecipeInput{
		RecipeTitle: "Chicken Fried Rice",
		Ingredients: []string{"chicken", "rice", "soy sauce", "egg"},
		LensNames:   []string{"Keto (Low-Carb)"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Compliant {
		t.Error("expected rice to be flagged non-compliant for Keto")
	}
	if !contains(result.Violations, "rice") {
		t.Errorf("expected 'rice' in violations, got %v", result.Violations)
	}

	result, err = r.CheckRecipeAgainstLens(CheckRecipeInput{
		RecipeTitle: "Avocado Egg Salad",
		Ingredients: []string{"eggs", "avocado", "olive oil", "leafy greens"},
		LensNames:   []string{"Keto (Low-Carb)"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Compliant {
		t.Errorf("expected avocado egg salad to be compliant for Keto, got violations: %v", result.Violations)
	}
}

func TestDiabeticFriendlyLensFlagsAddedSugar(t *testing.T) {
	r := NewRegistry()
	result, err := r.CheckRecipeAgainstLens(CheckRecipeInput{
		RecipeTitle: "Glazed Pastries",
		Ingredients: []string{"pastries", "added sugar", "white flour"},
		LensNames:   []string{"Diabetic-Friendly"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Compliant {
		t.Error("expected pastries/added sugar to be flagged non-compliant for Diabetic-Friendly")
	}
	if !contains(result.Violations, "pastries") || !contains(result.Violations, "added sugar") {
		t.Errorf("expected pastries and added sugar in violations, got %v", result.Violations)
	}
}

func TestVegetarianLensFlagsMeatAndFishButAllowsDairyAndEggs(t *testing.T) {
	r := NewRegistry()
	result, err := r.CheckRecipeAgainstLens(CheckRecipeInput{
		RecipeTitle: "Chicken Curry",
		Ingredients: []string{"chicken", "coconut milk", "curry paste"},
		LensNames:   []string{"Vegetarian"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Compliant {
		t.Error("expected chicken to be flagged non-compliant for Vegetarian")
	}
	if !contains(result.Violations, "chicken") {
		t.Errorf("expected 'chicken' in violations, got %v", result.Violations)
	}

	result, err = r.CheckRecipeAgainstLens(CheckRecipeInput{
		RecipeTitle: "Cheese and Spinach Frittata",
		Ingredients: []string{"eggs", "cheddar cheese", "spinach"},
		LensNames:   []string{"Vegetarian"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Compliant {
		t.Errorf("expected eggs/cheese to be compliant for Vegetarian, got violations: %v", result.Violations)
	}
}

func TestVeganLensFlagsDairyAndEggsToo(t *testing.T) {
	r := NewRegistry()
	result, err := r.CheckRecipeAgainstLens(CheckRecipeInput{
		RecipeTitle: "Cheese and Spinach Frittata",
		Ingredients: []string{"eggs", "cheddar cheese", "spinach"},
		LensNames:   []string{"Vegan"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Compliant {
		t.Error("expected eggs/cheese to be flagged non-compliant for Vegan")
	}
	if !contains(result.Violations, "eggs") || !contains(result.Violations, "cheese") {
		t.Errorf("expected eggs and cheese in violations, got %v", result.Violations)
	}

	result, err = r.CheckRecipeAgainstLens(CheckRecipeInput{
		RecipeTitle: "Tofu Stir-Fry",
		Ingredients: []string{"tofu", "broccoli", "bell peppers", "soy sauce"},
		LensNames:   []string{"Vegan"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Compliant {
		t.Errorf("expected tofu stir-fry to be compliant for Vegan, got violations: %v", result.Violations)
	}
}

func TestCombineLensesUnionsAvoidLists(t *testing.T) {
	r := NewRegistry()
	// Vegetarian + GERD: a recipe with chicken (Vegetarian violation) and
	// citrus (GERD violation) should be flagged for both, from one combined
	// check, not just whichever lens happened to be checked.
	result, err := r.CheckRecipeAgainstLens(CheckRecipeInput{
		RecipeTitle: "Citrus Chicken",
		Ingredients: []string{"chicken", "orange", "citrus zest"},
		LensNames:   []string{"Vegetarian", "GERD-Friendly"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Compliant {
		t.Error("expected non-compliant recipe")
	}
	if !contains(result.Violations, "chicken") {
		t.Errorf("expected 'chicken' (Vegetarian violation) in violations, got %v", result.Violations)
	}
	if !contains(result.Violations, "citrus") {
		t.Errorf("expected 'citrus' (GERD violation) in violations, got %v", result.Violations)
	}
	if result.LensName != "Vegetarian + GERD-Friendly" {
		t.Errorf("expected combined display name, got %q", result.LensName)
	}

	// A recipe compliant with both individually should be compliant combined.
	result, err = r.CheckRecipeAgainstLens(CheckRecipeInput{
		RecipeTitle: "Ginger Tofu Bowl",
		Ingredients: []string{"tofu", "ginger", "brown rice", "spinach"},
		LensNames:   []string{"Vegetarian", "GERD-Friendly"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Compliant {
		t.Errorf("expected compliant recipe, got violations: %v", result.Violations)
	}
}

func TestCombineLensesMacroTargetsFirstNonNilWins(t *testing.T) {
	r := NewRegistry()
	// Macro Target sets ProteinG=30; Vegetarian sets no macro targets at
	// all, so the combined lens should still carry that protein target
	// through regardless of order -- exactly the kind of combo (a pure
	// ingredient-exclusion lens plus a pure macro-target lens) splitting
	// the old bundled GERD+Hormonal+Macro preset into three was for.
	lens, err := r.CombineLenses("user-1", []string{"Vegetarian", "Macro Target (30g Protein / 40g Carbs / 15g Fat)"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lens.MacroTargets.ProteinG == nil || *lens.MacroTargets.ProteinG != 30 {
		t.Errorf("expected the macro lens's protein target (30) to carry through, got %v", lens.MacroTargets.ProteinG)
	}
}

func TestCombineLensesUnknownNameReturnsError(t *testing.T) {
	r := NewRegistry()
	_, err := r.CombineLenses("user-1", []string{"Vegetarian", "Not A Real Lens"})
	if err == nil {
		t.Error("expected an error when one of several lens names is unknown")
	}
}

func TestGetLensPresetUnknownNameReturnsError(t *testing.T) {
	r := NewRegistry()
	_, err := r.GetLensPreset("user-1", "Not A Real Lens")
	if err == nil {
		t.Error("expected an error for an unknown lens name")
	}
}

func TestGetLensPresetReturnsFullDefinition(t *testing.T) {
	r := NewRegistry()
	lens, err := r.GetLensPreset("user-1", "Athletic Performance")
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
		LensNames:   []string{"GERD-Friendly"},
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
		LensNames:   []string{"GERD-Friendly"},
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
		LensNames:   []string{"Athletic Performance"},
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
		LensNames:   []string{"Nonexistent Lens"},
	})
	if err == nil {
		t.Error("expected an error for an unknown lens")
	}
}

func TestSaveAndReuseCustomLens(t *testing.T) {
	r := NewRegistry()
	lowCal := 500
	lowProtein, lowCarbs, lowFat := 30, 50, 15
	r.SaveCustomLens("user-1", DietaryLens{
		Name:              "Low FODMAP",
		AvoidIngredients:  []string{"garlic", "onion", "wheat"},
		PreferIngredients: []string{"rice", "carrot", "chicken"},
		CustomRules:       "Avoid high-FODMAP fruits and legumes.",
		MacroTargets: MacroTargets{
			Calories: &lowCal, ProteinG: &lowProtein, CarbsG: &lowCarbs, FatG: &lowFat,
		},
	})

	names := map[string]bool{}
	for _, s := range r.ListLensPresets("user-1") {
		names[s.Name] = true
	}
	if !names["Low FODMAP"] {
		t.Fatal("expected 'Low FODMAP' to be listed after saving")
	}

	result, err := r.CheckRecipeAgainstLens(CheckRecipeInput{
		UserID:      "user-1",
		RecipeTitle: "Garlic Onion Soup",
		Ingredients: []string{"garlic", "onion", "vegetable broth"},
		LensNames:   []string{"Low FODMAP"},
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

func TestCustomLensesAreScopedPerUser(t *testing.T) {
	r := NewRegistry()
	r.SaveCustomLens("user-1", DietaryLens{Name: "Keto", CustomRules: "user-1's lens"})

	names := map[string]bool{}
	for _, s := range r.ListLensPresets("user-2") {
		names[s.Name] = true
	}
	if names["Keto"] {
		t.Error("user-2 should not see user-1's custom lens in ListLensPresets")
	}

	if _, err := r.GetLensPreset("user-2", "Keto"); err == nil {
		t.Error("user-2 should not be able to fetch user-1's custom lens via GetLensPreset")
	}

	// user-2 saving a same-named lens must not affect user-1's copy.
	r.SaveCustomLens("user-2", DietaryLens{Name: "Keto", CustomRules: "user-2's lens"})
	lens, err := r.GetLensPreset("user-1", "Keto")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lens.CustomRules != "user-1's lens" {
		t.Errorf("expected user-1's lens to be unaffected by user-2's save, got %q", lens.CustomRules)
	}
}
