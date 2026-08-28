// Package app wires the Recipe Concierge agent together: the Gemini model,
// the prompt, and the tools from tools_adk.go. It depends on
// pantrylens/core for the actual lens/compliance logic (see core/tools.go)
// and on google.golang.org/adk/v2 for the agent framework itself.
package app

import (
	"context"
	"fmt"
	"log"
	"os"

	"google.golang.org/genai"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model/gemini"

	"pantrylens/core"
)

// ModelName -- the hackathon requires Gemini 3.5 Flash or newer. This is
// the current default model name in the pinned google.golang.org/genai
// SDK's own example code (examples/models/generate_content/text_stream.go
// in v1.66.0), not a guess. Model availability can still vary by GCP
// project/region allowlist, so confirm it resolves against Vertex AI
// Model Garden in your own project before you submit -- an easy way to
// silently fail the "required technology" judging criterion is a wrong
// model string here. If it's unavailable, "gemini-3-flash-preview" is the
// fallback seen in the same SDK's docs; confirm it still satisfies
// "3.5 or newer" per the hackathon rules before switching to it.
const ModelName = "gemini-3.5-flash"

// NewRootAgent builds the Recipe Concierge agent.
//
// The empty &genai.ClientConfig{} below is deliberate: with no fields set,
// the client resolves its backend, project, and location from environment
// variables (GOOGLE_GENAI_USE_ENTERPRISE, GOOGLE_CLOUD_PROJECT,
// GOOGLE_CLOUD_LOCATION) -- the same Vertex AI setup documented in
// README.md "Using your $300 credit", so calls bill against your GCP
// project rather than a separate AI Studio quota.
func NewRootAgent(ctx context.Context) (agent.Agent, error) {
	model, err := gemini.NewModel(ctx, ModelName, &genai.ClientConfig{})
	if err != nil {
		return nil, fmt.Errorf("create model: %w", err)
	}

	tools, err := BuildTools(newRegistry(ctx))
	if err != nil {
		return nil, fmt.Errorf("build tools: %w", err)
	}

	rootAgent, err := llmagent.New(llmagent.Config{
		Name:  "recipe_concierge",
		Model: model,
		Description: "Collaborative recipe agent: turns available ingredients into recipes " +
			"that fit a configurable dietary lens, refined through conversation.",
		Instruction: RootAgentInstruction,
		Tools:       tools,
	})
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}
	return rootAgent, nil
}

// newRegistry builds a Firestore-backed lens registry when GOOGLE_CLOUD_PROJECT
// is set (so custom lenses survive process restarts, e.g. Cloud Run cold
// starts), falling back to an in-memory registry -- unchanged from local/
// offline dev today -- if that env var is unset or Firestore construction
// fails for any reason (auth, network, etc).
func newRegistry(ctx context.Context) *core.Registry {
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		return core.NewRegistry()
	}
	registry, err := core.NewFirestoreRegistry(ctx, projectID)
	if err != nil {
		log.Printf("Firestore registry unavailable (%v); falling back to in-memory", err)
		return core.NewRegistry()
	}
	return registry
}

// NewPreferenceStore builds a Firestore-backed preference store when
// GOOGLE_CLOUD_PROJECT is set, mirroring newRegistry's fallback-to-in-memory
// behavior. Exported (unlike newRegistry) because it's also needed outside
// the agent itself -- the frontend's own /profile endpoints read and write
// preferences directly, without going through the agent or its tools (see
// ../cmd/pantrylens/frontend/frontend.go), since they're plain structured
// fields the intake form prefills, not something the model reasons about.
func NewPreferenceStore(ctx context.Context) core.PreferenceStore {
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		return core.NewPreferenceStore()
	}
	store, err := core.NewFirestorePreferenceStore(ctx, projectID)
	if err != nil {
		log.Printf("Firestore preference store unavailable (%v); falling back to in-memory", err)
		return core.NewPreferenceStore()
	}
	return store
}

// NewRecipeStore builds a Firestore-backed recipe store when
// GOOGLE_CLOUD_PROJECT is set, mirroring NewPreferenceStore's
// fallback-to-in-memory behavior -- the frontend's own POST/GET /recipes
// endpoints save and serve a recipe's "open in a new tab" view directly,
// without going through the agent (see ../cmd/pantrylens/frontend
// /frontend.go).
func NewRecipeStore(ctx context.Context) core.RecipeStore {
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		return core.NewRecipeStore()
	}
	store, err := core.NewFirestoreRecipeStore(ctx, projectID)
	if err != nil {
		log.Printf("Firestore recipe store unavailable (%v); falling back to in-memory", err)
		return core.NewRecipeStore()
	}
	return store
}
