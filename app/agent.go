// Package app wires the Recipe Concierge agent together: the Gemini model,
// the prompt, and the tools from tools_adk.go. It depends on
// pantrylens/core for the actual lens/compliance logic (see core/tools.go)
// and on google.golang.org/adk/v2 for the agent framework itself.
package app

import (
	"context"
	"fmt"

	"google.golang.org/genai"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model/gemini"
)

// ModelName -- the hackathon requires Gemini 3.5 Flash or newer. This is a
// placeholder verified against nothing but the Gemini 2.0 naming
// convention; confirm the exact current Gemini 3.5 Flash model ID in the
// GCP Console's Model Garden before you rely on this, and definitely
// before you submit (an easy way to silently fail the "required
// technology" judging criterion is a wrong model string here).
const ModelName = "gemini-2.0-flash"

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

	tools, err := BuildTools()
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
