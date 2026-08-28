// Package core holds the framework-agnostic parts of PantryLens: the
// dietary lens model and the deterministic logic for checking a recipe
// against one. Nothing in this package imports ADK, Gemini, or any GCP
// client -- that's deliberate. It means this package builds and tests with
// nothing but the Go standard library (see tools_test.go), and the ADK
// wiring in ../app is a thin adapter around it, not the other way around.
package core

// MacroTargets holds rough per-serving macro targets. Zero value for any
// field means "no target set" -- use pointers so that's distinguishable
// from an explicit target of 0.
type MacroTargets struct {
	Calories *int
	ProteinG *int
	CarbsG   *int
	FatG     *int
}

// DietaryLens is a named, reusable set of dietary goals and constraints.
type DietaryLens struct {
	Name              string
	AvoidIngredients  []string
	PreferIngredients []string
	MacroTargets      MacroTargets
	CustomRules       string
	NotesStyle        string
}

func intPtr(v int) *int { return &v }

// Built-in presets, covering a deliberately wide mix to prove the lens
// system is generic, not tuned to a single person's diet: macro-target
// health/performance lenses (Hormonal Balance, Macro Target, Athletic,
// Diabetic-Friendly, Keto), and pure ingredient-exclusion lenses with no
// macro targets (GERD-Friendly, Vegetarian, Vegan, Heart-Healthy,
// Gluten-Free, Dairy-Free, Low-FODMAP, Kidney-Friendly) -- MacroTargets is
// left at its zero value ("no target set") for the latter group, since
// e.g. excluding animal products isn't a macro-balancing problem. A user
// can select more than one of these at once (see Registry.CombineLenses
// below) -- e.g. Vegetarian + GERD-Friendly + Macro Target -- rather than
// being limited to exactly one.
//
// AvoidIngredients below lists concrete terms likely to appear literally in
// a recipe's ingredient strings (e.g. "chicken", not "poultry") rather than
// abstract categories -- see the "Known limitation" note on mentions() in
// tools.go: matching is literal/keyword-based, not food-category knowledge,
// so an avoid-entry only catches ingredients that actually share a word
// with it.
var (
	// GERDFriendlyLens, HormonalBalanceLens, and MacroTargetLens used to be
	// one bundled "GERD + Hormonal Balance + Macro Target" preset -- split
	// into three so they're independently selectable and combinable (e.g.
	// Vegetarian + Macro Target, with no GERD restrictions at all), matching
	// how every other lens here works. They were never actually
	// interdependent; bundling them just meant an all-or-nothing choice
	// that fought the multi-select system the rest of these lenses use.
	GERDFriendlyLens = DietaryLens{
		Name:              "GERD-Friendly",
		AvoidIngredients:  []string{"caffeine", "citrus", "tomato", "fried foods", "alcohol", "spicy chili"},
		PreferIngredients: []string{"leafy greens", "ginger", "oats"},
		CustomRules:       "Keep meals low-acidity and anti-inflammatory to minimize reflux triggers.",
		NotesStyle:        "per-ingredient rationale",
	}

	HormonalBalanceLens = DietaryLens{
		Name:              "Hormonal Balance",
		AvoidIngredients:  []string{"added sugar", "white sugar"},
		PreferIngredients: []string{"flaxseed", "leafy greens", "oats", "whole grains"},
		MacroTargets: MacroTargets{
			CarbsG: intPtr(40),
		},
		CustomRules: "Favor fiber-rich, phytoestrogen-friendly foods and steady blood sugar " +
			"(moderate, complex carbs over refined sugar) to support hormonal balance.",
		NotesStyle: "per-ingredient rationale",
	}

	MacroTargetLens = DietaryLens{
		Name:              "Macro Target (30g Protein / 40g Carbs / 15g Fat)",
		PreferIngredients: []string{"lean protein", "complex carbohydrates"},
		MacroTargets: MacroTargets{
			ProteinG: intPtr(30),
			CarbsG:   intPtr(40),
			FatG:     intPtr(15),
		},
		CustomRules: "Hit these per-serving macro targets; no ingredient restrictions beyond that.",
		NotesStyle:  "brief",
	}

	AthleticPerformanceLens = DietaryLens{
		Name:              "Athletic Performance",
		AvoidIngredients:  []string{"deep-fried foods", "excess added sugar"},
		PreferIngredients: []string{"lean protein", "complex carbohydrates", "leafy greens", "fruit"},
		MacroTargets: MacroTargets{
			Calories: intPtr(650),
			ProteinG: intPtr(45),
			CarbsG:   intPtr(70),
			FatG:     intPtr(15),
		},
		CustomRules: "Optimize for post-workout recovery: high protein, fast-digesting carbs.",
		NotesStyle:  "brief",
	}

	VegetarianLens = DietaryLens{
		Name: "Vegetarian",
		AvoidIngredients: []string{
			"chicken", "beef", "pork", "turkey", "lamb", "bacon", "ham", "sausage",
			"fish", "shrimp", "salmon", "tuna", "gelatin",
		},
		PreferIngredients: []string{"tofu", "beans", "lentils", "chickpeas", "eggs", "dairy", "vegetables", "whole grains"},
		CustomRules: "Strictly vegetarian: no meat, poultry, fish, or seafood, and no gelatin. " +
			"Dairy and eggs are fine unless the user says otherwise.",
		NotesStyle: "brief",
	}

	VeganLens = DietaryLens{
		Name: "Vegan",
		AvoidIngredients: []string{
			"chicken", "beef", "pork", "turkey", "lamb", "bacon", "ham", "sausage",
			"fish", "shrimp", "salmon", "tuna", "gelatin",
			"dairy", "milk", "cheese", "butter", "cream", "yogurt", "eggs", "honey",
		},
		PreferIngredients: []string{"tofu", "tempeh", "beans", "lentils", "chickpeas", "nuts", "seeds", "vegetables", "whole grains", "plant milk"},
		CustomRules: "Strictly vegan: no meat, poultry, fish, seafood, dairy, eggs, honey, or any " +
			"other animal-derived ingredient.",
		NotesStyle: "brief",
	}

	DiabeticFriendlyLens = DietaryLens{
		Name: "Diabetic-Friendly",
		AvoidIngredients: []string{
			"white sugar", "added sugar", "white bread", "white rice", "candy", "soda", "fruit juice", "syrup", "pastries",
		},
		PreferIngredients: []string{"whole grains", "leafy greens", "legumes", "lean protein", "nuts", "non-starchy vegetables"},
		MacroTargets: MacroTargets{
			CarbsG: intPtr(45),
		},
		CustomRules: "Prioritize low glycemic-index, high-fiber carbohydrates for steady blood sugar; " +
			"minimize refined sugar and simple starches. This is general dietary guidance, not " +
			"personalized medical advice.",
		NotesStyle: "per-ingredient rationale",
	}

	HeartHealthyLens = DietaryLens{
		Name: "Heart-Healthy (Low-Sodium)",
		AvoidIngredients: []string{
			"salt", "bacon", "processed meat", "deli meat", "canned soup", "soy sauce", "butter", "fried foods",
		},
		PreferIngredients: []string{"olive oil", "leafy greens", "whole grains", "fish", "beans", "fresh herbs"},
		CustomRules: "Keep sodium and saturated fat low; favor heart-healthy fats (olive oil, nuts, " +
			"fatty fish) and fresh, unprocessed ingredients over salty or canned/processed ones.",
		NotesStyle: "brief",
	}

	GlutenFreeLens = DietaryLens{
		Name:              "Gluten-Free",
		AvoidIngredients:  []string{"wheat", "barley", "rye", "regular pasta", "bread", "soy sauce", "flour", "couscous", "beer"},
		PreferIngredients: []string{"rice", "quinoa", "corn", "gluten-free oats", "potatoes", "vegetables"},
		CustomRules: "Strictly gluten-free: no wheat, barley, rye, or anything made from them " +
			"(regular bread, pasta, soy sauce, beer). Suggest gluten-free substitutes.",
		NotesStyle: "brief",
	}

	DairyFreeLens = DietaryLens{
		Name:              "Dairy-Free",
		AvoidIngredients:  []string{"milk", "cheese", "butter", "cream", "yogurt", "whey", "ice cream"},
		PreferIngredients: []string{"plant milk", "coconut milk", "dairy-free cheese", "olive oil", "nuts"},
		CustomRules: "Strictly dairy-free: no milk, cheese, butter, cream, or yogurt. Suggest " +
			"plant-based substitutes.",
		NotesStyle: "brief",
	}

	KetoLens = DietaryLens{
		Name:              "Keto (Low-Carb)",
		AvoidIngredients:  []string{"rice", "pasta", "bread", "potatoes", "sugar", "beans", "corn", "oats"},
		PreferIngredients: []string{"eggs", "cheese", "avocado", "olive oil", "leafy greens", "meat", "fish", "nuts"},
		MacroTargets: MacroTargets{
			ProteinG: intPtr(25),
			CarbsG:   intPtr(20),
			FatG:     intPtr(40),
		},
		CustomRules: "Strict low-carb, high-fat: minimize starches, grains, sugar, and high-carb " +
			"vegetables/fruits; favor healthy fats and moderate protein.",
		NotesStyle: "per-ingredient rationale",
	}

	LowFODMAPLens = DietaryLens{
		Name:              "Low-FODMAP",
		AvoidIngredients:  []string{"garlic", "onion", "wheat", "beans", "lentils", "apples", "honey", "milk", "cashews"},
		PreferIngredients: []string{"rice", "quinoa", "carrots", "spinach", "chicken", "eggs", "lactose-free milk"},
		CustomRules: "Low-FODMAP: avoid high-FODMAP triggers like garlic, onion, wheat, legumes, and " +
			"certain fruits/dairy; favor low-FODMAP swaps (e.g. garlic-infused oil instead of garlic).",
		NotesStyle: "per-ingredient rationale",
	}

	KidneyFriendlyLens = DietaryLens{
		Name: "Kidney-Friendly (Renal)",
		AvoidIngredients: []string{
			"bananas", "potatoes", "tomatoes", "oranges", "dairy", "beans", "nuts", "dark leafy greens", "processed meat",
		},
		PreferIngredients: []string{"rice", "white bread", "apples", "berries", "cabbage", "cauliflower"},
		CustomRules: "Kidney-friendly (renal diet): keep potassium, phosphorus, and sodium low, and " +
			"keep protein moderate. Renal diets are highly individual and stage-dependent -- flag " +
			"portion sizes and remind the user to confirm specifics with their care team, this is " +
			"general guidance, not personalized medical advice.",
		NotesStyle: "per-ingredient rationale",
	}
)

// BuiltInLenses returns a fresh copy of the built-in lens registry, keyed
// by name. Returning a fresh map each call keeps callers from accidentally
// mutating the shared presets above.
func BuiltInLenses() map[string]DietaryLens {
	lenses := []DietaryLens{
		GERDFriendlyLens,
		HormonalBalanceLens,
		MacroTargetLens,
		AthleticPerformanceLens,
		VegetarianLens,
		VeganLens,
		DiabeticFriendlyLens,
		HeartHealthyLens,
		GlutenFreeLens,
		DairyFreeLens,
		KetoLens,
		LowFODMAPLens,
		KidneyFriendlyLens,
	}
	byName := make(map[string]DietaryLens, len(lenses))
	for _, l := range lenses {
		byName[l.Name] = l
	}
	return byName
}
