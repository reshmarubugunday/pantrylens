// Package frontend is PantryLens's own consumer-facing web UI -- a small,
// static chat page (not ADK's built-in developer console, which surfaces
// Events/Traces/State panels aimed at debugging agents, not at end users).
// It's registered as a "ui" sublauncher of ADK's web launcher, alongside
// the "api" sublauncher (google.golang.org/adk/v2/cmd/launcher/web/api)
// it talks to via same-origin fetch calls to /api/... -- see static/app.js.
// Being same-origin means no CORS configuration is needed, unlike ADK's
// own webui sublauncher (see README.md's now-removed ADK_PUBLIC_URL note).
package frontend

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"google.golang.org/adk/v2/cmd/launcher"
	weblauncher "google.golang.org/adk/v2/cmd/launcher/web"

	"pantrylens/core"
)

//go:embed static
var staticFiles embed.FS

// maxPhotoBytes caps uploads to /detect-ingredients -- generous enough for
// a real phone-camera photo, small enough to keep a stray huge upload from
// tying up the request or ballooning the Gemini call's cost.
const maxPhotoBytes = 8 << 20 // 8 MiB

type sublauncher struct {
	preferences core.PreferenceStore
	detector    core.IngredientDetector // nil if photo detection isn't configured (see NewLauncher)
	recipes     core.RecipeStore
}

// NewLauncher returns a web.Sublauncher that serves PantryLens's static
// frontend at "/", plus several small PantryLens-owned REST endpoints
// (deliberately outside "/api" and not routed through the agent or its
// tools -- see ../../tools_adk.go -- since none of them need the
// conversational tool loop): GET/PUT /profile/{userID} for prefilling and
// remembering servings/cuisine, POST /detect-ingredients for photo-based
// ingredient detection, and POST /recipes + GET /recipes?userId=... + GET
// /recipes/{id} for saving a recipe, listing "My saved recipes" for a
// user, and viewing one specific saved recipe's own standalone page (see
// static/app.js for all of these). Saves are scoped to userID (the same
// anonymous, device-local ID the profile endpoints already use -- there's
// no login in this app) so a listing only shows what that browser saved,
// but a specific recipe's view link works for anyone who has it,
// regardless of who saved it -- same as a shared Google Doc link. detector
// may be nil (e.g. Gemini vision unavailable in this environment) --
// /detect-ingredients then responds 503 rather than the server failing to
// start over a feature that's allowed to be missing. Register this
// sublauncher in web.NewLauncher after api.NewLauncher() -- SetupSubrouters
// is called in registration order, and this sublauncher's catch-all "/"
// route must not be set up before api's more specific "/api" prefix, or it
// would shadow it.
func NewLauncher(preferences core.PreferenceStore, detector core.IngredientDetector, recipes core.RecipeStore) weblauncher.Sublauncher {
	return &sublauncher{preferences: preferences, detector: detector, recipes: recipes}
}

func (s *sublauncher) Keyword() string { return "ui" }

func (s *sublauncher) Parse(args []string) ([]string, error) { return args, nil }

func (s *sublauncher) CommandLineSyntax() string { return "" }

func (s *sublauncher) SimpleDescription() string {
	return "starts PantryLens's own web UI (a small chat page, not ADK's dev console)"
}

func (s *sublauncher) SetupSubrouters(router *mux.Router, _ *launcher.Config) error {
	// Registered before the catch-all file server below, since gorilla/mux
	// matches routes in registration order and the catch-all's GET
	// PathPrefix("/") would otherwise shadow the profile GET route too.
	router.HandleFunc("/profile/{userID}", s.getProfile).Methods(http.MethodGet)
	router.HandleFunc("/profile/{userID}", s.putProfile).Methods(http.MethodPut)
	router.HandleFunc("/detect-ingredients", s.detectIngredients).Methods(http.MethodPost)
	router.HandleFunc("/recipes", s.saveRecipe).Methods(http.MethodPost)
	router.HandleFunc("/recipes", s.listRecipes).Methods(http.MethodGet)
	router.HandleFunc("/recipes/{id}", s.viewRecipe).Methods(http.MethodGet)

	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return err
	}
	router.Methods(http.MethodGet).PathPrefix("/").Handler(http.FileServer(http.FS(sub)))
	return nil
}

// profileBody is the JSON shape of both the GET response and PUT request
// body for /profile/{userID}.
type profileBody struct {
	LastServings int    `json:"lastServings,omitempty"`
	LastCuisine  string `json:"lastCuisine,omitempty"`
}

// getProfile returns the user's last-saved preferences, or the zero value
// (servings 0, cuisine "") if they haven't saved any yet -- the frontend
// treats a zero/empty value as "nothing to prefill", same as a fresh visit.
func (s *sublauncher) getProfile(w http.ResponseWriter, r *http.Request) {
	userID := mux.Vars(r)["userID"]
	prefs, _ := s.preferences.Get(userID)
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	json.NewEncoder(w).Encode(profileBody{LastServings: prefs.LastServings, LastCuisine: prefs.LastCuisine})
}

func (s *sublauncher) putProfile(w http.ResponseWriter, r *http.Request) {
	userID := mux.Vars(r)["userID"]
	var body profileBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}
	s.preferences.Save(userID, core.Preferences{LastServings: body.LastServings, LastCuisine: body.LastCuisine})
	w.WriteHeader(http.StatusNoContent)
}

type detectIngredientsBody struct {
	Ingredients []string `json:"ingredients"`
}

// detectIngredients accepts a raw image (any Content-Type the browser's
// File.type gives us -- see static/app.js; falls back to sniffing if that
// header is missing or generic) and returns the ingredients Gemini can
// identify in it. Errors from the model call itself are logged and
// returned as 502s rather than crashing the request handler -- a bad photo
// or a model hiccup shouldn't take down anything else.
func (s *sublauncher) detectIngredients(w http.ResponseWriter, r *http.Request) {
	if s.detector == nil {
		http.Error(w, "photo-based ingredient detection isn't configured on this server", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoBytes)
	imageBytes, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "photo is too large (8 MiB max)", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "couldn't read the uploaded photo: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(imageBytes) == 0 {
		http.Error(w, "no photo data received", http.StatusBadRequest)
		return
	}

	mimeType := r.Header.Get("Content-Type")
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(imageBytes)
	}

	ingredients, err := s.detector.Detect(r.Context(), imageBytes, mimeType)
	if err != nil {
		http.Error(w, "couldn't analyze that photo: "+err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	json.NewEncoder(w).Encode(detectIngredientsBody{Ingredients: ingredients})
}

// saveRecipeBody is the JSON shape POST /recipes accepts -- most field
// names and json tags match propose_recipe's args exactly (see
// ../../tools_adk.go's proposeRecipeArgs), since the frontend already has
// that exact object in hand from the propose_recipe functionCall event
// (see static/app.js's addRecipeCard) and can POST most of it as-is.
// UserID and MealPrepBatch are the two exceptions -- frontend-computed
// extras with nothing to do with the agent or propose_recipe.
type saveRecipeBody struct {
	// UserID is the anonymous, device-local ID from localStorage (see
	// static/app.js) -- required so the save can be indexed for that
	// user's "My saved recipes" list.
	UserID                string   `json:"userId"`
	Title                 string   `json:"title"`
	Cuisine               string   `json:"cuisine,omitempty"`
	MealType              string   `json:"mealType,omitempty"`
	Servings              int      `json:"servings,omitempty"`
	Ingredients           []string `json:"ingredients,omitempty"`
	Steps                 []string `json:"steps,omitempty"`
	Calories              *int     `json:"calories,omitempty"`
	ProteinG              *int     `json:"proteinG,omitempty"`
	CarbsG                *int     `json:"carbsG,omitempty"`
	FatG                  *int     `json:"fatG,omitempty"`
	LensNote              string   `json:"lensNote,omitempty"`
	StorageNote           string   `json:"storageNote,omitempty"`
	AdditionalIngredients []string `json:"additionalIngredients,omitempty"`
	// MealPrepBatch, when set, is a shared label the frontend generates
	// once per meal-prep response (see static/app.js's "Save all" button)
	// so every recipe saved from that batch groups together in "My saved
	// recipes" -- see core.SavedRecipe.MealPrepBatch.
	MealPrepBatch string `json:"mealPrepBatch,omitempty"`
}

type saveRecipeResponse struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// newRecipeID mints a random, unguessable ID for a saved recipe's URL --
// there's no listing endpoint, so the ID itself is the only thing standing
// between "anyone with the link" and "anyone at all."
func newRecipeID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (s *sublauncher) saveRecipe(w http.ResponseWriter, r *http.Request) {
	var body saveRecipeBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}
	if body.UserID == "" {
		http.Error(w, "userId is required", http.StatusBadRequest)
		return
	}

	id, err := newRecipeID()
	if err != nil {
		http.Error(w, "couldn't generate a recipe ID: "+err.Error(), http.StatusInternalServerError)
		return
	}

	recipe := core.SavedRecipe{
		Title:                 body.Title,
		Cuisine:               body.Cuisine,
		MealType:              body.MealType,
		Servings:              body.Servings,
		Ingredients:           body.Ingredients,
		Steps:                 body.Steps,
		Calories:              body.Calories,
		ProteinG:              body.ProteinG,
		CarbsG:                body.CarbsG,
		FatG:                  body.FatG,
		LensNote:              body.LensNote,
		StorageNote:           body.StorageNote,
		AdditionalIngredients: body.AdditionalIngredients,
		MealPrepBatch:         body.MealPrepBatch,
	}
	if err := s.recipes.Save(body.UserID, id, recipe); err != nil {
		http.Error(w, "couldn't save that recipe: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	json.NewEncoder(w).Encode(saveRecipeResponse{ID: id, URL: "/recipes/" + id})
}

type savedRecipeListItem struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	URL           string    `json:"url"`
	SavedAt       time.Time `json:"savedAt"`
	MealPrepBatch string    `json:"mealPrepBatch,omitempty"`
	MealType      string    `json:"mealType,omitempty"`
}

type listRecipesResponse struct {
	Recipes []savedRecipeListItem `json:"recipes"`
}

// listRecipes serves "My saved recipes" (see static/app.js) -- GET
// /recipes?userId=... , most recently saved first (see RecipeStore.List).
// An empty/missing userId just yields an empty list rather than an error,
// same treatment as getProfile gives a user with nothing saved yet.
func (s *sublauncher) listRecipes(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("userId")
	summaries := s.recipes.List(userID)
	items := make([]savedRecipeListItem, 0, len(summaries))
	for _, sum := range summaries {
		items = append(items, savedRecipeListItem{
			ID:            sum.ID,
			Title:         sum.Title,
			URL:           "/recipes/" + sum.ID,
			SavedAt:       sum.SavedAt,
			MealPrepBatch: sum.MealPrepBatch,
			MealType:      sum.MealType,
		})
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	json.NewEncoder(w).Encode(listRecipesResponse{Recipes: items})
}

func (s *sublauncher) viewRecipe(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	recipe, ok := s.recipes.Get(id)
	if !ok {
		http.Error(w, "no saved recipe found at this link", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := recipeViewTmpl.Execute(w, toRecipeViewData(recipe)); err != nil {
		// Execute may have already written a partial body by the time it
		// fails, so http.Error here would just append garbled text after
		// a half-rendered page rather than fixing anything -- log instead.
		log.Printf("viewRecipe: render %s: %v", id, err)
	}
}

func (s *sublauncher) UserMessage(webURL string, printer func(v ...any)) {
	printer("        ui:  " + webURL + "/")
}
