package app

// This file is the thin ADK adapter layer: it wraps the framework-agnostic
// logic in pantrylens/core as ADK tools. Every handler below does almost
// nothing itself -- it converts between the tool's argument/result structs
// and core's plain Go types, and delegates the actual work to the
// *core.Registry passed into BuildTools (see agent.go's newRegistry,
// which picks in-memory vs Firestore-backed). Keeping that split means
// core stays testable without ADK, Gemini, or a network connection (see
// core/tools_test.go), and this file stays small enough to read in one pass.
//
// NOTE: this file cannot be compiled in the environment PantryLens was
// scaffolded in -- google.golang.org/adk/v2 requires Go 1.26+ and open
// network access to fetch, neither of which that sandbox had. It's written
// directly against the real, current ADK Go API (verified by reading the
// google/adk-go source, specifically tool/functiontool/function.go and
// examples/tools/multipletools/main.go), not guessed. Run `go mod tidy`
// on your own machine to pull in the real dependency versions and confirm
// it compiles -- see README.md.

import (
	"pantrylens/core"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
)

// --- list_lens_presets -------------------------------------------------

type listLensPresetsArgs struct{}

type listLensPresetsResult struct {
	Lenses []core.LensSummary `json:"lenses" jsonschema:"the available dietary lenses, built-in and custom"`
}

func newListLensPresetsTool(registry *core.Registry) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name: "list_lens_presets",
		Description: "List all available dietary lenses (built-in, plus any custom ones this " +
			"user has saved previously), so the agent can describe the options to the user.",
	}, func(ctx agent.Context, _ listLensPresetsArgs) (listLensPresetsResult, error) {
		return listLensPresetsResult{Lenses: registry.ListLensPresets(ctx.UserID())}, nil
	})
}

// --- get_lens_preset -----------------------------------------------------

type getLensPresetArgs struct {
	Names []string `json:"names" jsonschema:"one or more exact lens names, as returned by list_lens_presets. Pass all of the user's currently active lenses together (e.g. [\"Vegetarian\", \"GERD-Friendly\"]) to get back one merged view of every rule that applies at once, rather than calling this once per lens."`
}

type lensOutput struct {
	Name              string   `json:"name" jsonschema:"the lens name, or -- if multiple names were requested -- all of their names joined with ' + ', representing their merged rules"`
	AvoidIngredients  []string `json:"avoidIngredients" jsonschema:"ingredients/categories to avoid entirely"`
	PreferIngredients []string `json:"preferIngredients" jsonschema:"ingredients/categories to favor when there's a choice"`
	Calories          *int     `json:"calories,omitempty" jsonschema:"target calories per serving"`
	ProteinG          *int     `json:"proteinG,omitempty" jsonschema:"target grams of protein per serving"`
	CarbsG            *int     `json:"carbsG,omitempty" jsonschema:"target grams of carbohydrates per serving"`
	FatG              *int     `json:"fatG,omitempty" jsonschema:"target grams of fat per serving"`
	CustomRules       string   `json:"customRules" jsonschema:"free-text rules the agent should follow, one bracketed [Lens Name] block per requested lens"`
	NotesStyle        string   `json:"notesStyle" jsonschema:"how to explain ingredient choices to the user"`
}

func toLensOutput(l core.DietaryLens) lensOutput {
	return lensOutput{
		Name:              l.Name,
		AvoidIngredients:  l.AvoidIngredients,
		PreferIngredients: l.PreferIngredients,
		Calories:          l.MacroTargets.Calories,
		ProteinG:          l.MacroTargets.ProteinG,
		CarbsG:            l.MacroTargets.CarbsG,
		FatG:              l.MacroTargets.FatG,
		CustomRules:       l.CustomRules,
		NotesStyle:        l.NotesStyle,
	}
}

func newGetLensPresetTool(registry *core.Registry) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name: "get_lens_preset",
		Description: "Fetch the full definition of one or more dietary lenses by name. When the " +
			"user has more than one active lens (e.g. Vegetarian + GERD-Friendly), pass all their names in " +
			"one call to get back the combined rule set -- see CombineLenses in core/tools.go for " +
			"exactly how avoid/prefer lists, macro targets, and custom rules get merged.",
	}, func(ctx agent.Context, args getLensPresetArgs) (lensOutput, error) {
		lens, err := registry.CombineLenses(ctx.UserID(), args.Names)
		if err != nil {
			return lensOutput{}, err
		}
		return toLensOutput(lens), nil
	})
}

// --- save_custom_lens ------------------------------------------------

type saveCustomLensArgs struct {
	Name              string   `json:"name" jsonschema:"a short, human-readable name for the new lens"`
	AvoidIngredients  []string `json:"avoidIngredients" jsonschema:"ingredients/categories to avoid entirely"`
	PreferIngredients []string `json:"preferIngredients" jsonschema:"ingredients/categories to favor"`
	CustomRules       string   `json:"customRules" jsonschema:"free-text rules to follow, e.g. low sodium, no added sugar"`
	Calories          *int     `json:"calories,omitempty" jsonschema:"optional target calories per serving"`
	ProteinG          *int     `json:"proteinG,omitempty" jsonschema:"optional target grams of protein per serving"`
	CarbsG            *int     `json:"carbsG,omitempty" jsonschema:"optional target grams of carbohydrates per serving"`
	FatG              *int     `json:"fatG,omitempty" jsonschema:"optional target grams of fat per serving"`
}

type saveCustomLensResult struct {
	Saved string `json:"saved" jsonschema:"the name of the lens that was saved"`
}

func newSaveCustomLensTool(registry *core.Registry) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name: "save_custom_lens",
		Description: "Create (or overwrite) a custom dietary lens for this user. Use this " +
			"when the user describes dietary goals that don't match an existing preset, so this " +
			"and future conversations with this user can reuse it by name.",
	}, func(ctx agent.Context, args saveCustomLensArgs) (saveCustomLensResult, error) {
		registry.SaveCustomLens(ctx.UserID(), core.DietaryLens{
			Name:              args.Name,
			AvoidIngredients:  args.AvoidIngredients,
			PreferIngredients: args.PreferIngredients,
			CustomRules:       args.CustomRules,
			MacroTargets: core.MacroTargets{
				Calories: args.Calories,
				ProteinG: args.ProteinG,
				CarbsG:   args.CarbsG,
				FatG:     args.FatG,
			},
		})
		return saveCustomLensResult{Saved: args.Name}, nil
	})
}

// --- check_recipe_against_lens ------------------------------------------

type checkRecipeArgs struct {
	RecipeTitle       string   `json:"recipeTitle" jsonschema:"the recipe's title, for reference in the result"`
	Ingredients       []string `json:"ingredients" jsonschema:"the recipe's ingredient list (plain text entries)"`
	LensNames         []string `json:"lensNames" jsonschema:"the name(s) of every currently active lens to check against, see list_lens_presets. Pass all of them together (e.g. [\"Vegetarian\", \"GERD-Friendly\"]) when the user has more than one active -- a recipe must satisfy all of them, not just one."`
	Calories          *int     `json:"calories,omitempty" jsonschema:"the recipe's estimated calories per serving, if known"`
	ProteinG          *int     `json:"proteinG,omitempty" jsonschema:"the recipe's estimated grams of protein per serving, if known"`
	CarbsG            *int     `json:"carbsG,omitempty" jsonschema:"the recipe's estimated grams of carbohydrates per serving, if known"`
	FatG              *int     `json:"fatG,omitempty" jsonschema:"the recipe's estimated grams of fat per serving, if known"`
	MacroTolerancePct int      `json:"macroTolerancePct,omitempty" jsonschema:"how far macros may drift from the lens's targets (percent) before being flagged as a warning; defaults to 25"`
}

func newCheckRecipeAgainstLensTool(registry *core.Registry) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name: "check_recipe_against_lens",
		Description: "Validate a proposed recipe against one or more active dietary lenses before " +
			"presenting it to the user -- when several are active at once (e.g. Vegetarian + GERD-Friendly), " +
			"pass all their names in lensNames and the recipe must satisfy every one of them, not just " +
			"whichever lens happens to be checked. Always call this after drafting a candidate recipe " +
			"and before showing it to the user -- it performs a deterministic check (not another model " +
			"call), so violations are caught reliably.",
	}, func(ctx agent.Context, args checkRecipeArgs) (core.CheckRecipeResult, error) {
		return registry.CheckRecipeAgainstLens(core.CheckRecipeInput{
			UserID:            ctx.UserID(),
			RecipeTitle:       args.RecipeTitle,
			Ingredients:       args.Ingredients,
			LensNames:         args.LensNames,
			Calories:          args.Calories,
			ProteinG:          args.ProteinG,
			CarbsG:            args.CarbsG,
			FatG:              args.FatG,
			MacroTolerancePct: args.MacroTolerancePct,
		})
	})
}

// --- propose_recipe -------------------------------------------------------

type proposeRecipeArgs struct {
	Title       string   `json:"title" jsonschema:"the recipe's title"`
	Cuisine     string   `json:"cuisine,omitempty" jsonschema:"the recipe's cuisine style (e.g. Italian, Thai), if known or requested by the user"`
	MealType    string   `json:"mealType,omitempty" jsonschema:"which meal this recipe is for -- exactly one of 'Breakfast', 'Lunch', 'Dinner', or 'Snack'. Use the user's stated meal type verbatim if they gave one. If they didn't, you may fill in whichever of the four the recipe itself unambiguously is (e.g. an omelette is 'Breakfast'), but leave this empty rather than guessing for a recipe that's genuinely fine at more than one (e.g. a grain bowl that works for lunch or dinner)."`
	Servings    int      `json:"servings,omitempty" jsonschema:"how many servings this recipe (and its ingredient quantities) is scaled for"`
	Ingredients []string `json:"ingredients" jsonschema:"the recipe's ingredient list, with rough quantities scaled to servings"`
	Steps       []string `json:"steps" jsonschema:"the recipe's numbered preparation steps, in order"`
	Calories    *int     `json:"calories,omitempty" jsonschema:"estimated calories per serving"`
	ProteinG    *int     `json:"proteinG,omitempty" jsonschema:"estimated grams of protein per serving"`
	CarbsG      *int     `json:"carbsG,omitempty" jsonschema:"estimated grams of carbohydrates per serving"`
	FatG        *int     `json:"fatG,omitempty" jsonschema:"estimated grams of fat per serving"`
	LensNote    string   `json:"lensNote,omitempty" jsonschema:"one-line note on why this recipe fits the active dietary lens"`
	StorageNote string   `json:"storageNote,omitempty" jsonschema:"for meal-prep batches only: one-line note on how this recipe stores and reheats (e.g. 'fridge up to 4 days, microwave 2 min'); leave empty for a single tonight's-dinner recipe"`
	// AdditionalIngredients is the shopping-list subset of Ingredients --
	// see prompts.go step 2 for exactly what counts (not in the user's
	// on-hand list, and not a pantry staple already being assumed).
	AdditionalIngredients []string `json:"additionalIngredients,omitempty" jsonschema:"the subset of ingredients (copied verbatim from the ingredients list, quantity and all) that the user did NOT say they have on hand and will need to buy or get separately -- excluding common pantry staples you're already assuming (salt, oil, water, pepper, etc). Leave empty if every ingredient came from what the user listed."`
}

type proposeRecipeResult struct {
	Acknowledged bool `json:"acknowledged"`
}

func newProposeRecipeTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name: "propose_recipe",
		Description: "Present one candidate recipe to the user in structured form, so the UI can " +
			"render it as a recipe card. Call this once per recipe, immediately after it has passed " +
			"check_recipe_against_lens with no unresolved violations -- never call this for an " +
			"unvalidated recipe. This carries the full recipe details (ingredients, steps, macros); " +
			"your own chat response should stay brief and conversational rather than repeating them. " +
			"For a meal-prep batch, call this once per recipe in the batch and fill in storageNote.",
	}, func(_ agent.Context, args proposeRecipeArgs) (proposeRecipeResult, error) {
		return proposeRecipeResult{Acknowledged: true}, nil
	})
}

// BuildTools constructs every tool the Recipe Concierge agent uses, backed
// by the given lens registry (see core.NewRegistry / core.NewFirestoreRegistry).
// Saving a recipe to view later isn't one of these -- see
// ../cmd/pantrylens/frontend/frontend.go's POST /recipes -- since the
// recipe's full details are already in the frontend's hands the moment
// propose_recipe carries them there, so saving it is a plain structured
// write with nothing for the agent to reason about.
func BuildTools(registry *core.Registry) ([]tool.Tool, error) {
	var tools []tool.Tool
	for _, build := range []func() (tool.Tool, error){
		func() (tool.Tool, error) { return newListLensPresetsTool(registry) },
		func() (tool.Tool, error) { return newGetLensPresetTool(registry) },
		func() (tool.Tool, error) { return newSaveCustomLensTool(registry) },
		func() (tool.Tool, error) { return newCheckRecipeAgainstLensTool(registry) },
		newProposeRecipeTool,
	} {
		t, err := build()
		if err != nil {
			return nil, err
		}
		tools = append(tools, t)
	}
	return tools, nil
}
