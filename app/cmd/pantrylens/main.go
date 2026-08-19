// Command pantrylens launches the Recipe Concierge agent via ADK's
// "full" launcher, which gives you both a console chat mode and a local
// web UI from the same binary -- the Go equivalent of Python's `adk run`
// and `adk web`.
//
// Usage (after `go mod tidy` succeeds -- see ../../README.md):
//
//	go run . console   # chat with the agent in your terminal
//	go run . web       # local web UI, prints a URL to open
package main

import (
	"context"
	"log"
	"os"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/cmd/launcher"
	"google.golang.org/adk/v2/cmd/launcher/full"

	"pantrylens/app"
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

	l := full.NewLauncher()
	if err := l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}
