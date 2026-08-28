// Command pantrylens launches the Recipe Concierge agent, either as a
// terminal chat (console mode) or as a small web server serving
// PantryLens's own chat frontend (see ./frontend) backed by ADK's REST API
// -- deliberately not ADK's built-in webui sublauncher, which is a
// developer console (Events/Traces/State panels) rather than something
// meant for an end user to see.
//
// Usage (after `go mod tidy` succeeds -- see ../../README.md):
//
//	go run . console      # chat with the agent in your terminal
//	go run . web ui api   # local web server: PantryLens's UI + its REST API
package main

import (
	"context"
	"log"
	"os"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/cmd/launcher"
	"google.golang.org/adk/v2/cmd/launcher/console"
	"google.golang.org/adk/v2/cmd/launcher/universal"
	"google.golang.org/adk/v2/cmd/launcher/web"
	"google.golang.org/adk/v2/cmd/launcher/web/api"

	"pantrylens/app"
	"pantrylens/app/cmd/pantrylens/frontend"
)

func main() {
	ctx := context.Background()

	rootAgent, err := app.NewRootAgent(ctx)
	if err != nil {
		log.Fatalf("failed to build agent: %v", err)
	}

	config := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(rootAgent),
	}

	// Photo-based ingredient detection is allowed to be unavailable (e.g.
	// no Gemini credentials in this environment) without the whole server
	// failing to start -- frontend.NewLauncher treats a nil detector as
	// "feature off" and returns a clean 503 from /detect-ingredients.
	detector, err := app.NewIngredientDetector(ctx)
	if err != nil {
		log.Printf("photo-based ingredient detection unavailable (%v); continuing without it", err)
	}

	// api must come before frontend.NewLauncher() here: SetupSubrouters runs
	// in this slice's order, and frontend's catch-all "/" route would shadow
	// api's more specific "/api" prefix if registered first.
	l := universal.NewLauncher(console.NewLauncher(), web.NewLauncher(api.NewLauncher(), frontend.NewLauncher(app.NewPreferenceStore(ctx), detector, app.NewRecipeStore(ctx))))
	if err := l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}
