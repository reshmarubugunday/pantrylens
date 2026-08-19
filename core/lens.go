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

// Built-in presets. Two deliberately different ones ship by default to
// prove the lens system is generic, not tuned to a single person's diet.
var (
	GERDHormoneMacroLens = DietaryLens{
		Name:              "GERD + Hormonal Balance + Macro Target",
		AvoidIngredients:  []string{"caffeine", "citrus", "tomato", "fried foods", "alcohol", "spicy chili"},
		PreferIngredients: []string{"leafy greens", "lean protein", "ginger", "oats", "flaxseed"},
		MacroTargets: MacroTargets{
			ProteinG: intPtr(30),
			CarbsG:   intPtr(40),
			FatG:     intPtr(15),
		},
		CustomRules: "Keep meals low-acidity and anti-inflammatory. Favor foods that support " +
			"hormonal balance (fiber-rich, steady blood sugar, phytoestrogen-friendly).",
		NotesStyle: "per-ingredient rationale",
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
)

// BuiltInLenses returns a fresh copy of the built-in lens registry, keyed
// by name. Returning a fresh map each call keeps callers from accidentally
// mutating the shared presets above.
func BuiltInLenses() map[string]DietaryLens {
	return map[string]DietaryLens{
		GERDHormoneMacroLens.Name:    GERDHormoneMacroLens,
		AthleticPerformanceLens.Name: AthleticPerformanceLens,
	}
}
