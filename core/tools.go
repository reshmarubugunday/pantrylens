package core

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// Registry holds custom, user-defined lenses created during a session, on
// top of the built-in presets. It's safe for concurrent use.
//
// This is an in-memory stand-in for what should be a Firestore-backed store
// once this moves past local testing (see the build plan's Day 4) -- the
// method signatures below are written so that swap doesn't touch callers.
type Registry struct {
	mu     sync.RWMutex
	custom map[string]DietaryLens
}

// NewRegistry returns a registry seeded with just the built-in presets.
func NewRegistry() *Registry {
	return &Registry{custom: make(map[string]DietaryLens)}
}

func (r *Registry) all() map[string]DietaryLens {
	all := BuiltInLenses()
	r.mu.RLock()
	defer r.mu.RUnlock()
	for name, lens := range r.custom {
		all[name] = lens
	}
	return all
}

// LensSummary is the shape returned by ListLensPresets -- enough for an
// agent to describe available lenses without dumping the full definition.
type LensSummary struct {
	Name        string `json:"name"`
	CustomRules string `json:"customRules"`
}

// ListLensPresets lists all available dietary lenses, built-in and custom.
func (r *Registry) ListLensPresets() []LensSummary {
	all := r.all()
	summaries := make([]LensSummary, 0, len(all))
	for _, lens := range all {
		summaries = append(summaries, LensSummary{Name: lens.Name, CustomRules: lens.CustomRules})
	}
	return summaries
}

// GetLensPreset fetches the full definition of a lens by exact name.
func (r *Registry) GetLensPreset(name string) (DietaryLens, error) {
	all := r.all()
	lens, ok := all[name]
	if !ok {
		return DietaryLens{}, fmt.Errorf("no lens named %q; call ListLensPresets to see options", name)
	}
	return lens, nil
}

// SaveCustomLens creates or overwrites a custom lens for this registry.
func (r *Registry) SaveCustomLens(lens DietaryLens) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.custom[lens.Name] = lens
}

// CheckRecipeInput is the input to CheckRecipeAgainstLens.
type CheckRecipeInput struct {
	RecipeTitle string
	Ingredients []string
	LensName    string
	Calories    *int
	ProteinG    *int
	CarbsG      *int
	FatG        *int
	// MacroTolerancePct is how far macros may drift from the lens's targets
	// (as a percentage) before being flagged as a warning. Zero means "use
	// the default" (25).
	MacroTolerancePct int
}

// CheckRecipeResult is the result of CheckRecipeAgainstLens.
type CheckRecipeResult struct {
	RecipeTitle string   `json:"recipeTitle"`
	LensName    string   `json:"lensName"`
	Compliant   bool     `json:"compliant"`
	Violations  []string `json:"violations"`
	Warnings    []string `json:"warnings"`
}

var wordRE = regexp.MustCompile(`[a-z]+`)

var stopwords = map[string]bool{
	"and": true, "or": true, "foods": true, "food": true,
	"added": true, "excess": true, "the": true, "a": true,
}

// tokens extracts lowercase word tokens from text, dropping stopwords.
func tokens(text string) map[string]bool {
	out := make(map[string]bool)
	for _, w := range wordRE.FindAllString(strings.ToLower(text), -1) {
		if !stopwords[w] {
			out[w] = true
		}
	}
	return out
}

func tokensOverlap(a, b map[string]bool) bool {
	// Iterate the smaller set for a cheap overlap check.
	small, big := a, b
	if len(b) < len(a) {
		small, big = b, a
	}
	for t := range small {
		if big[t] {
			return true
		}
	}
	return false
}

// mentions reports whether phrase is present in ingredients, either as a
// literal substring of some ingredient, or via shared significant words
// (so an avoid-entry of "spicy chili" still catches "chili flakes").
//
// Known limitation, called out on purpose: this is a literal/keyword match,
// not food-category knowledge. An avoid entry of "citrus" won't infer that
// "orange juice" is a citrus product -- that needs a synonym/category table
// (or a second model call), which is out of scope for this scaffold. The
// agent's own instructions (see ../app/prompts.go) are what's relied on to
// avoid citrus-family ingredients in the first place; this is a safety net,
// not a substitute for that judgment.
func mentions(phrase string, ingredients []string, ingredientTokens map[string]bool) bool {
	phraseLower := strings.ToLower(phrase)
	for _, ing := range ingredients {
		if strings.Contains(strings.ToLower(ing), phraseLower) {
			return true
		}
	}
	return tokensOverlap(tokens(phrase), ingredientTokens)
}

func macroWarning(value, target *int, label string, tolerancePct int) string {
	if value == nil || target == nil || *target == 0 {
		return ""
	}
	diff := *value - *target
	if diff < 0 {
		diff = -diff
	}
	driftPct := float64(diff) / float64(*target) * 100
	if driftPct > float64(tolerancePct) {
		return fmt.Sprintf("%s is %d, target is %d (off by %.0f%%).", label, *value, *target, driftPct)
	}
	return ""
}

// CheckRecipeAgainstLens validates a proposed recipe against a named
// dietary lens. Always call this after drafting a candidate recipe and
// before showing it to the user -- it's a deterministic check (not another
// model call), so violations are caught reliably rather than relying on
// the model's own judgment every time.
func (r *Registry) CheckRecipeAgainstLens(in CheckRecipeInput) (CheckRecipeResult, error) {
	lens, err := r.GetLensPreset(in.LensName)
	if err != nil {
		return CheckRecipeResult{}, err
	}

	tolerance := in.MacroTolerancePct
	if tolerance == 0 {
		tolerance = 25
	}

	ingredientTokens := make(map[string]bool)
	for _, ing := range in.Ingredients {
		for t := range tokens(ing) {
			ingredientTokens[t] = true
		}
	}

	var violations []string
	for _, banned := range lens.AvoidIngredients {
		if mentions(banned, in.Ingredients, ingredientTokens) {
			violations = append(violations, banned)
		}
	}

	var warnings []string
	allPreferredUnused := len(lens.PreferIngredients) > 0
	for _, preferred := range lens.PreferIngredients {
		if mentions(preferred, in.Ingredients, ingredientTokens) {
			allPreferredUnused = false
			break
		}
	}
	if allPreferredUnused {
		warnings = append(warnings, fmt.Sprintf(
			"None of the preferred ingredients (%s) are used.",
			strings.Join(lens.PreferIngredients, ", ")))
	}

	for _, w := range []string{
		macroWarning(in.Calories, lens.MacroTargets.Calories, "Calories", tolerance),
		macroWarning(in.ProteinG, lens.MacroTargets.ProteinG, "Protein (g)", tolerance),
		macroWarning(in.CarbsG, lens.MacroTargets.CarbsG, "Carbs (g)", tolerance),
		macroWarning(in.FatG, lens.MacroTargets.FatG, "Fat (g)", tolerance),
	} {
		if w != "" {
			warnings = append(warnings, w)
		}
	}

	return CheckRecipeResult{
		RecipeTitle: in.RecipeTitle,
		LensName:    in.LensName,
		Compliant:   len(violations) == 0,
		Violations:  violations,
		Warnings:    warnings,
	}, nil
}
