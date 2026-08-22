package app

// This test operationalizes the "architectural discipline" pitch as a
// real, runnable check rather than a claim: every candidate recipe must be
// run through check_recipe_against_lens before the agent finishes
// responding. It drives the real agent (Gemini via Vertex AI), so it's
// skipped unless GOOGLE_CLOUD_PROJECT is set -- it costs real API quota
// and needs live credentials, and `go test ./...` must stay green for
// anyone without GCP access.

import (
	"context"
	"os"
	"testing"

	"google.golang.org/genai"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
)

func TestComplianceCheckIsCalledBeforeRecipesAreShown(t *testing.T) {
	if os.Getenv("GOOGLE_CLOUD_PROJECT") == "" {
		t.Skip("GOOGLE_CLOUD_PROJECT not set; skipping live-model regression test")
	}

	ctx := context.Background()
	rootAgent, err := NewRootAgent(ctx)
	if err != nil {
		t.Fatalf("NewRootAgent: %v", err)
	}

	const appName, userID = "pantrylens_test", "test_user"
	sessionService := session.InMemoryService()
	created, err := sessionService.Create(ctx, &session.CreateRequest{AppName: appName, UserID: userID})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	r, err := runner.New(runner.Config{AppName: appName, Agent: rootAgent, SessionService: sessionService})
	if err != nil {
		t.Fatalf("runner.New: %v", err)
	}

	msg := genai.NewContentFromText(
		"I have chicken breast, spinach, and oats. Use the Athletic Performance lens and suggest a recipe.",
		genai.RoleUser,
	)

	sawComplianceCheck := false
	for event, err := range r.Run(ctx, userID, created.Session.ID(), msg, agent.RunConfig{
		StreamingMode: agent.StreamingModeNone,
	}) {
		if err != nil {
			t.Fatalf("agent run: %v", err)
		}
		if event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if fc := part.FunctionCall; fc != nil && fc.Name == "check_recipe_against_lens" {
				sawComplianceCheck = true
			}
		}
	}

	if !sawComplianceCheck {
		t.Error("expected check_recipe_against_lens to be called at least once, but it wasn't -- " +
			"the agent may have presented a recipe without validating it against the active lens")
	}
}
