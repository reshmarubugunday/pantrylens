package core

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// LensStore is where a Registry keeps custom, user-defined lenses (the
// built-in presets always come from BuiltInLenses() regardless of store),
// scoped per user so two different callers' custom lenses never collide or
// overwrite each other. The default is an in-memory map (see
// newInMemoryLensStore); see firestore_store.go for a Firestore-backed
// implementation and NewFirestoreRegistry, used so custom lenses survive
// process restarts (e.g. Cloud Run cold starts) once this moves past local
// testing.
type LensStore interface {
	List(userID string) []DietaryLens
	Get(userID, name string) (DietaryLens, bool)
	Save(userID string, lens DietaryLens)
}

// Registry holds custom, user-defined lenses created during a session, on
// top of the built-in presets. It's shared by every user of the process --
// callers must pass the calling user's ID into every method so custom
// lenses stay scoped to the user who created them; the built-in presets
// are global and returned to everyone regardless of ID. It's safe for
// concurrent use as long as its LensStore is (both implementations in this
// package are).
type Registry struct {
	store LensStore
}

// NewRegistry returns a registry backed by an in-memory store, seeded with
// just the built-in presets.
func NewRegistry() *Registry {
	return &Registry{store: newInMemoryLensStore()}
}

func (r *Registry) all(userID string) map[string]DietaryLens {
	all := BuiltInLenses()
	for _, lens := range r.store.List(userID) {
		all[lens.Name] = lens
	}
	return all
}

type inMemoryLensStore struct {
	mu     sync.RWMutex
	custom map[string]map[string]DietaryLens // userID -> lens name -> lens
}

func newInMemoryLensStore() *inMemoryLensStore {
	return &inMemoryLensStore{custom: make(map[string]map[string]DietaryLens)}
}

func (s *inMemoryLensStore) List(userID string) []DietaryLens {
	s.mu.RLock()
	defer s.mu.RUnlock()
	byName := s.custom[userID]
	lenses := make([]DietaryLens, 0, len(byName))
	for _, lens := range byName {
		lenses = append(lenses, lens)
	}
	return lenses
}

func (s *inMemoryLensStore) Get(userID, name string) (DietaryLens, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	lens, ok := s.custom[userID][name]
	return lens, ok
}

func (s *inMemoryLensStore) Save(userID string, lens DietaryLens) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.custom[userID] == nil {
		s.custom[userID] = make(map[string]DietaryLens)
	}
	s.custom[userID][lens.Name] = lens
}

// LensSummary is the shape returned by ListLensPresets -- enough for an
// agent to describe available lenses without dumping the full definition.
type LensSummary struct {
	Name        string `json:"name"`
	CustomRules string `json:"customRules"`
}

// ListLensPresets lists all dietary lenses available to userID: every
// built-in preset plus that user's own custom lenses (not other users').
func (r *Registry) ListLensPresets(userID string) []LensSummary {
	all := r.all(userID)
	summaries := make([]LensSummary, 0, len(all))
	for _, lens := range all {
		summaries = append(summaries, LensSummary{Name: lens.Name, CustomRules: lens.CustomRules})
	}
	return summaries
}

// GetLensPreset fetches the full definition of a lens by exact name, from
// userID's own custom lenses or the built-in presets.
func (r *Registry) GetLensPreset(userID, name string) (DietaryLens, error) {
	all := r.all(userID)
	lens, ok := all[name]
	if !ok {
		return DietaryLens{}, fmt.Errorf("no lens named %q; call ListLensPresets to see options", name)
	}
	return lens, nil
}

// SaveCustomLens creates or overwrites a custom lens, scoped to userID.
func (r *Registry) SaveCustomLens(userID string, lens DietaryLens) {
	r.store.Save(userID, lens)
}

// CombineLenses merges one or more named lenses (built-in and/or userID's
// own custom ones) into a single synthesized DietaryLens representing "all
// of these must hold at once" -- e.g. Vegetarian + GERD -- so a recipe can
// be drafted and checked against the whole combination in one pass instead
// of the agent juggling several separate lenses itself. It's a pure,
// on-the-fly composition: the result is never saved back into the
// registry, and names(1) degenerates to plain GetLensPreset.
//
// The merge rules, applied field by field across all named lenses in
// order:
//   - AvoidIngredients / PreferIngredients: union, de-duplicated -- avoid
//     anything any selected lens avoids; prefer anything any of them
//     prefers.
//   - MacroTargets: first non-nil value wins per field (Calories, ProteinG,
//     CarbsG, FatG independently) -- if two selected lenses both set a
//     macro target, the first-listed one's target applies for that field,
//     rather than attempting to average or reconcile genuinely conflicting
//     numeric targets.
//   - CustomRules: every non-empty rule text is kept, each prefixed with
//     its lens's name in brackets, so the model sees exactly which rule
//     came from which lens rather than an unattributed merged blob.
//   - NotesStyle: "per-ingredient rationale" wins over "brief" if any
//     selected lens asks for it -- more thorough is the safer default when
//     merging several sets of constraints into one recipe.
//   - Name: every selected lens's name, joined with " + ".
func (r *Registry) CombineLenses(userID string, names []string) (DietaryLens, error) {
	if len(names) == 0 {
		return DietaryLens{}, fmt.Errorf("at least one lens name is required")
	}
	if len(names) == 1 {
		return r.GetLensPreset(userID, names[0])
	}

	all := r.all(userID)
	var combined DietaryLens
	var displayNames, ruleParts []string
	avoidSeen := map[string]bool{}
	preferSeen := map[string]bool{}

	for _, name := range names {
		lens, ok := all[name]
		if !ok {
			return DietaryLens{}, fmt.Errorf("no lens named %q; call ListLensPresets to see options", name)
		}
		displayNames = append(displayNames, lens.Name)

		for _, a := range lens.AvoidIngredients {
			if !avoidSeen[a] {
				avoidSeen[a] = true
				combined.AvoidIngredients = append(combined.AvoidIngredients, a)
			}
		}
		for _, p := range lens.PreferIngredients {
			if !preferSeen[p] {
				preferSeen[p] = true
				combined.PreferIngredients = append(combined.PreferIngredients, p)
			}
		}

		if combined.MacroTargets.Calories == nil {
			combined.MacroTargets.Calories = lens.MacroTargets.Calories
		}
		if combined.MacroTargets.ProteinG == nil {
			combined.MacroTargets.ProteinG = lens.MacroTargets.ProteinG
		}
		if combined.MacroTargets.CarbsG == nil {
			combined.MacroTargets.CarbsG = lens.MacroTargets.CarbsG
		}
		if combined.MacroTargets.FatG == nil {
			combined.MacroTargets.FatG = lens.MacroTargets.FatG
		}

		if lens.CustomRules != "" {
			ruleParts = append(ruleParts, fmt.Sprintf("[%s] %s", lens.Name, lens.CustomRules))
		}
		if lens.NotesStyle == "per-ingredient rationale" {
			combined.NotesStyle = "per-ingredient rationale"
		} else if combined.NotesStyle == "" {
			combined.NotesStyle = lens.NotesStyle
		}
	}

	combined.Name = strings.Join(displayNames, " + ")
	combined.CustomRules = strings.Join(ruleParts, " ")
	return combined, nil
}

// CheckRecipeInput is the input to CheckRecipeAgainstLens.
type CheckRecipeInput struct {
	// UserID scopes which caller's custom lenses LensNames may resolve to,
	// on top of the built-in presets available to everyone.
	UserID      string
	RecipeTitle string
	Ingredients []string
	// LensNames is one or more active lens names, checked jointly -- see
	// Registry.CombineLenses. A single name behaves exactly as before.
	LensNames []string
	Calories  *int
	ProteinG  *int
	CarbsG    *int
	FatG      *int
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

// CheckRecipeAgainstLens validates a proposed recipe against one or more
// active dietary lenses (see CombineLenses). Always call this after
// drafting a candidate recipe and before showing it to the user -- it's a
// deterministic check (not another model call), so violations are caught
// reliably rather than relying on the model's own judgment every time.
func (r *Registry) CheckRecipeAgainstLens(in CheckRecipeInput) (CheckRecipeResult, error) {
	lens, err := r.CombineLenses(in.UserID, in.LensNames)
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
		LensName:    lens.Name,
		Compliant:   len(violations) == 0,
		Violations:  violations,
		Warnings:    warnings,
	}, nil
}
