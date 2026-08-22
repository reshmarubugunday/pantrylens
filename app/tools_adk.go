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
		Description: "List all available dietary lenses (built-in and any custom ones " +
			"created this session), so the agent can describe the options to the user.",
	}, func(_ agent.Context, _ listLensPresetsArgs) (listLensPresetsResult, error) {
		return listLensPresetsResult{Lenses: registry.ListLensPresets()}, nil
	})
}

// --- get_lens_preset -----------------------------------------------------

type getLensPresetArgs struct {
	Name string `json:"name" jsonschema:"the exact lens name, as returned by list_lens_presets"`
}

type lensOutput struct {
	Name              string   `json:"name" jsonschema:"lens name"`
	AvoidIngredients  []string `json:"avoidIngredients" jsonschema:"ingredients/categories to avoid entirely"`
	PreferIngredients []string `json:"preferIngredients" jsonschema:"ingredients/categories to favor when there's a choice"`
	Calories          *int     `json:"calories,omitempty" jsonschema:"target calories per serving"`
	ProteinG          *int     `json:"proteinG,omitempty" jsonschema:"target grams of protein per serving"`
	CarbsG            *int     `json:"carbsG,omitempty" jsonschema:"target grams of carbohydrates per serving"`
	FatG              *int     `json:"fatG,omitempty" jsonschema:"target grams of fat per serving"`
	CustomRules       string   `json:"customRules" jsonschema:"free-text rules the agent should follow"`
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
		Name:        "get_lens_preset",
		Description: "Fetch the full definition of a dietary lens by name.",
	}, func(_ agent.Context, args getLensPresetArgs) (lensOutput, error) {
		lens, err := registry.GetLensPreset(args.Name)
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
		Description: "Create (or overwrite) a custom dietary lens for this session. Use this " +
			"when the user describes dietary goals that don't match an existing preset, so future " +
			"recipe generation and compliance checks in this conversation can reuse it by name.",
	}, func(_ agent.Context, args saveCustomLensArgs) (saveCustomLensResult, error) {
		registry.SaveCustomLens(core.DietaryLens{
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
	LensName          string   `json:"lensName" jsonschema:"the name of the lens to check against, see list_lens_presets"`
	Calories          *int     `json:"calories,omitempty" jsonschema:"the recipe's estimated calories per serving, if known"`
	ProteinG          *int     `json:"proteinG,omitempty" jsonschema:"the recipe's estimated grams of protein per serving, if known"`
	CarbsG            *int     `json:"carbsG,omitempty" jsonschema:"the recipe's estimated grams of carbohydrates per serving, if known"`
	FatG              *int     `json:"fatG,omitempty" jsonschema:"the recipe's estimated grams of fat per serving, if known"`
	MacroTolerancePct int      `json:"macroTolerancePct,omitempty" jsonschema:"how far macros may drift from the lens's targets (percent) before being flagged as a warning; defaults to 25"`
}

func newCheckRecipeAgainstLensTool(registry *core.Registry) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name: "check_recipe_against_lens",
		Description: "Validate a proposed recipe against a named dietary lens before presenting " +
			"it to the user. Always call this after drafting a candidate recipe and before showing " +
			"it to the user -- it performs a deterministic check (not another model call), so " +
			"violations are caught reliably.",
	}, func(_ agent.Context, args checkRecipeArgs) (core.CheckRecipeResult, error) {
		return registry.CheckRecipeAgainstLens(core.CheckRecipeInput{
			RecipeTitle:       args.RecipeTitle,
			Ingredients:       args.Ingredients,
			LensName:          args.LensName,
			Calories:          args.Calories,
			ProteinG:          args.ProteinG,
			CarbsG:            args.CarbsG,
			FatG:              args.FatG,
			MacroTolerancePct: args.MacroTolerancePct,
		})
	})
}

// --- export_recipe_to_doc -----------------------------------------------

type exportRecipeToDocArgs struct {
	Title       string   `json:"title" jsonschema:"the recipe's title"`
	Ingredients []string `json:"ingredients" jsonschema:"the recipe's ingredient list, with quantities"`
	Steps       []string `json:"steps" jsonschema:"the recipe's numbered steps, in order"`
	Notes       string   `json:"notes,omitempty" jsonschema:"optional notes, e.g. why this recipe fits the active dietary lens"`
}

type exportRecipeToDocResult struct {
	DocURL string `json:"docUrl" jsonschema:"the shareable edit URL of the newly created Google Doc"`
}

func newExportRecipeToDocTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name: "export_recipe_to_doc",
		Description: "Export a finalized recipe to a new Google Doc and return its shareable link. " +
			"Only call this for a recipe the user has confirmed they want, and that already passed " +
			"check_recipe_against_lens -- never export an unvalidated draft. Offer this after the " +
			"user seems happy with a recipe; don't call it automatically on every revision.",
	}, func(ctx agent.Context, args exportRecipeToDocArgs) (exportRecipeToDocResult, error) {
		docURL, err := exportRecipeToDoc(ctx, FinalizedRecipe{
			Title:       args.Title,
			Ingredients: args.Ingredients,
			Steps:       args.Steps,
			Notes:       args.Notes,
		})
		if err != nil {
			return exportRecipeToDocResult{}, err
		}
		return exportRecipeToDocResult{DocURL: docURL}, nil
	})
}

// BuildTools constructs every tool the Recipe Concierge agent uses, backed
// by the given lens registry (see core.NewRegistry / core.NewFirestoreRegistry).
func BuildTools(registry *core.Registry) ([]tool.Tool, error) {
	var tools []tool.Tool
	for _, build := range []func() (tool.Tool, error){
		func() (tool.Tool, error) { return newListLensPresetsTool(registry) },
		func() (tool.Tool, error) { return newGetLensPresetTool(registry) },
		func() (tool.Tool, error) { return newSaveCustomLensTool(registry) },
		func() (tool.Tool, error) { return newCheckRecipeAgainstLensTool(registry) },
		newExportRecipeToDocTool,
	} {
		t, err := build()
		if err != nil {
			return nil, err
		}
		tools = append(tools, t)
	}
	return tools, nil
}
