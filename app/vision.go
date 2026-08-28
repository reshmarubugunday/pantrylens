package app

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/genai"

	"pantrylens/core"
)

// visionPrompt asks Gemini to name edible ingredients visible in a photo,
// in the same short, generic form a user would otherwise type by hand into
// the intake form's ingredient field (see static/app.js) -- "eggs", not "a
// carton of grade-A eggs, half used." Kept as its own constant for the same
// reason as RootAgentInstruction (see prompts.go): easy to iterate on
// without touching the plumbing around it.
const visionPrompt = `You are looking at a photo of a fridge, pantry, or kitchen counter.
Identify every distinct edible ingredient or food item you can clearly make
out. For each one, use a short, generic name suitable for a recipe
ingredient list -- e.g. "eggs", "spinach", "cheddar cheese" -- not a brand
name, a full product description, or a quantity. Ignore anything that
isn't food (containers, appliances, utensils) unless the food itself is
only identifiable by its packaging (e.g. "milk" from a milk carton). If
you're not reasonably confident an item is what you think it is, leave it
out rather than guessing. Return an empty list if you can't identify
anything with confidence -- don't invent items that aren't visible.`

type detectIngredientsResponse struct {
	Items []string `json:"items"`
}

// geminiIngredientDetector implements core.IngredientDetector using a
// direct, one-shot call to Gemini's vision capability -- deliberately not
// routed through the conversational agent in agent.go/tools_adk.go, since
// this has nothing to do with the chat/tool loop and runs before any
// session exists (see ../cmd/pantrylens/frontend/frontend.go).
type geminiIngredientDetector struct {
	client *genai.Client
}

// NewIngredientDetector builds a core.IngredientDetector backed by Gemini,
// using the same env-var-based Vertex/API-key auto-configuration as
// NewRootAgent's model (see the comment there).
func NewIngredientDetector(ctx context.Context) (core.IngredientDetector, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{})
	if err != nil {
		return nil, fmt.Errorf("create genai client: %w", err)
	}
	return &geminiIngredientDetector{client: client}, nil
}

func (d *geminiIngredientDetector) Detect(ctx context.Context, imageBytes []byte, mimeType string) ([]string, error) {
	contents := []*genai.Content{
		genai.NewContentFromParts([]*genai.Part{
			genai.NewPartFromText(visionPrompt),
			genai.NewPartFromBytes(imageBytes, mimeType),
		}, genai.RoleUser),
	}
	cfg := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"items": {
					Type:  genai.TypeArray,
					Items: &genai.Schema{Type: genai.TypeString},
				},
			},
			Required: []string{"items"},
		},
	}

	resp, err := d.client.Models.GenerateContent(ctx, ModelName, contents, cfg)
	if err != nil {
		return nil, fmt.Errorf("generate content: %w", err)
	}

	var out detectIngredientsResponse
	if err := json.Unmarshal([]byte(resp.Text()), &out); err != nil {
		return nil, fmt.Errorf("decode model response: %w", err)
	}
	return out.Items, nil
}
