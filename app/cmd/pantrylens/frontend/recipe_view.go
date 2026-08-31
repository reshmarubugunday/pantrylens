package frontend

import (
	"fmt"
	"html/template"
	"net/url"

	"pantrylens/core"
)

// recipeViewTemplate renders a saved recipe as its own standalone page --
// what "Save & view" (see static/app.js) opens in a new tab. It
// deliberately reuses styles.css's .recipe-card/.macro-chip/.lens-note/
// .storage-note classes verbatim (see static/app.js's addRecipeCard,
// which builds the same structure client-side for the in-chat card) so
// this page looks like the exact same card the user already saw, not a
// bare, differently-styled document -- and it's why this links to
// "/styles.css" rather than embedding its own copy: same origin, same
// stylesheet, automatically stays in sync with the live theme. All
// user/model-originated text below goes through html/template's
// auto-escaping, since recipe content ultimately comes from the model.
const recipeViewTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>{{.Title}} · PantryLens</title>
<link rel="icon" href="data:image/svg+xml,<svg xmlns=%22http://www.w3.org/2000/svg%22 viewBox=%220 0 100 100%22><text y=%22.9em%22 font-size=%2290%22>🥗</text></svg>" />
<link rel="stylesheet" href="/styles.css" />
</head>
<body>
<div class="app">
  <header class="app-header">
    <div class="brand">
      <span class="brand-mark" aria-hidden="true">🥗</span>
      <span class="brand-name">PantryLens</span>
    </div>
    <div class="header-actions">
      <a href="/" class="btn-ghost">Open PantryLens</a>
    </div>
  </header>
  <main class="chat" style="padding:24px 18px;">
    <div class="recipe-card">
      <div class="recipe-card-header">
        <h3>{{.Title}}</h3>
        {{if .Cuisine}}<span class="cuisine-badge">{{.Cuisine}}</span>{{end}}
        {{if .MealType}}<span class="cuisine-badge">{{.MealType}}</span>{{end}}
        {{if .ServingsLabel}}<span class="cuisine-badge">{{.ServingsLabel}}</span>{{end}}
        {{if .MealPrepBatch}}<span class="cuisine-badge">🍱 {{.MealPrepBatch}}</span>{{end}}
      </div>
      {{if .MacroChips}}
      <div class="macro-row">
        {{range .MacroChips}}<span class="macro-chip">{{.}}</span>{{end}}
      </div>
      {{end}}
      {{if .AdditionalIngredients}}
      <p class="shopping-note">🛒 You'll need to pick up: {{range $i, $ing := .AdditionalIngredients}}{{if $i}}, {{end}}{{$ing}}{{end}}</p>
      {{end}}
      <div class="recipe-body">
        {{if .Ingredients}}
        <h4>Ingredients</h4>
        <ul>{{range .Ingredients}}<li{{if contains $.AdditionalIngredients .}} class="need-to-buy"{{end}}>{{.}}{{if contains $.AdditionalIngredients .}} <span class="need-to-buy-tag">need to buy</span>{{end}}</li>{{end}}</ul>
        {{end}}
        {{if .Steps}}
        <h4>Steps</h4>
        <ol>{{range .Steps}}<li>{{.}}</li>{{end}}</ol>
        {{end}}
      </div>
      {{if .LensNote}}<p class="lens-note">✓ {{.LensNote}}</p>{{end}}
      {{if .StorageNote}}<p class="storage-note">🧊 {{.StorageNote}}</p>{{end}}
      {{if .VideoSearchURL}}<p class="video-link"><a href="{{.VideoSearchURL}}" target="_blank" rel="noopener">🎥 Watch recipe videos on YouTube</a></p>{{end}}
    </div>
  </main>
</div>
</body>
</html>
`

var recipeViewTmpl = template.Must(template.New("recipe").Funcs(template.FuncMap{
	// contains backs both the "need to buy" <li> tag and (indirectly, via
	// $.AdditionalIngredients being non-empty) the shopping-note summary --
	// html/template has no built-in slice-membership check.
	"contains": func(list []string, item string) bool {
		for _, s := range list {
			if s == item {
				return true
			}
		}
		return false
	},
}).Parse(recipeViewTemplate))

// recipeViewData is recipeViewTmpl's input -- a light reshaping of
// core.SavedRecipe into strings the template can drop in directly, so the
// template itself stays free of formatting logic.
type recipeViewData struct {
	Title                 string
	Cuisine               string
	MealType              string
	ServingsLabel         string
	Ingredients           []string
	Steps                 []string
	MacroChips            []string
	LensNote              string
	StorageNote           string
	AdditionalIngredients []string
	MealPrepBatch         string
	VideoSearchURL        string
}

// youTubeSearchURL builds a YouTube search-results link for a recipe title
// -- deliberately a search page, not a lookup for one specific video: no
// API key, no quota, and it never 404s the way pinning to one video ID
// would if that video were ever taken down. static/app.js's addRecipeCard
// builds the identical URL client-side for the in-chat card, since it has
// the title before any save/view round-trip happens.
func youTubeSearchURL(title string) string {
	if title == "" {
		return ""
	}
	return "https://www.youtube.com/results?search_query=" + url.QueryEscape(title+" recipe")
}

// toRecipeViewData mirrors static/app.js's macroChip()/addRecipeCard
// formatting exactly, so the standalone view matches the in-chat card.
func toRecipeViewData(r core.SavedRecipe) recipeViewData {
	data := recipeViewData{
		Title:                 r.Title,
		Cuisine:               r.Cuisine,
		MealType:              r.MealType,
		Ingredients:           r.Ingredients,
		Steps:                 r.Steps,
		LensNote:              r.LensNote,
		StorageNote:           r.StorageNote,
		AdditionalIngredients: r.AdditionalIngredients,
		MealPrepBatch:         r.MealPrepBatch,
		VideoSearchURL:        youTubeSearchURL(r.Title),
	}
	if r.Servings > 0 {
		unit := "servings"
		if r.Servings == 1 {
			unit = "serving"
		}
		data.ServingsLabel = fmt.Sprintf("%d %s", r.Servings, unit)
	}
	if r.Calories != nil {
		data.MacroChips = append(data.MacroChips, fmt.Sprintf("%d kcal", *r.Calories))
	}
	if r.ProteinG != nil {
		data.MacroChips = append(data.MacroChips, fmt.Sprintf("%dg protein", *r.ProteinG))
	}
	if r.CarbsG != nil {
		data.MacroChips = append(data.MacroChips, fmt.Sprintf("%dg carbs", *r.CarbsG))
	}
	if r.FatG != nil {
		data.MacroChips = append(data.MacroChips, fmt.Sprintf("%dg fat", *r.FatG))
	}
	return data
}
