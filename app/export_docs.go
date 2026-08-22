package app

// This file is the "real action" tool for the Collaborative Partner pitch:
// it turns a finalized, already-compliance-checked recipe into a real,
// shareable Google Doc via the Docs API, rather than leaving the recipe as
// chat text. It's a 100% I/O adapter (no deterministic business logic to
// keep framework-agnostic), so unlike the lens-compliance checker it lives
// here in app, not in core.
//
// Known scope boundary, accepted rather than solved: docs.NewService(ctx)
// resolves credentials the same way gemini.NewModel does (application
// default credentials). Under your own `gcloud auth application-default
// login` user credentials (local `console`/`web`), the created doc is
// immediately visible and editable by you. Under a Cloud Run service
// account, the doc would be owned by that service account and invisible
// to a human unless a Drive permission is explicitly granted afterward --
// this tool doesn't do that, so demo this feature from a local run, not
// from a Cloud Run deployment. Also note: the `cloud-platform` scope from
// a plain `gcloud auth application-default login` doesn't reliably cover
// Docs/Drive; if you hit a 403, re-run with
// `--scopes=https://www.googleapis.com/auth/cloud-platform,https://www.googleapis.com/auth/documents,https://www.googleapis.com/auth/drive.file`.

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/api/docs/v1"
)

// FinalizedRecipe is the minimal shape needed to export a recipe to a doc.
type FinalizedRecipe struct {
	Title       string
	Ingredients []string
	Steps       []string
	Notes       string
}

// exportRecipeToDoc creates a new Google Doc containing the recipe and
// returns its shareable edit URL.
func exportRecipeToDoc(ctx context.Context, recipe FinalizedRecipe) (string, error) {
	svc, err := docs.NewService(ctx)
	if err != nil {
		return "", fmt.Errorf("create docs client: %w", err)
	}

	doc, err := svc.Documents.Create(&docs.Document{Title: recipe.Title}).Do()
	if err != nil {
		return "", fmt.Errorf("create document: %w", err)
	}

	_, err = svc.Documents.BatchUpdate(doc.DocumentId, &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{
			{
				InsertText: &docs.InsertTextRequest{
					EndOfSegmentLocation: &docs.EndOfSegmentLocation{},
					Text:                 recipeDocBody(recipe),
				},
			},
		},
	}).Do()
	if err != nil {
		return "", fmt.Errorf("write document body: %w", err)
	}

	return fmt.Sprintf("https://docs.google.com/document/d/%s/edit", doc.DocumentId), nil
}

// recipeDocBody renders the recipe as plain text for a single InsertText
// request. Formatting (headings, bold, bullets) is left as a follow-up --
// plain, readable text is enough for a v1 export.
func recipeDocBody(recipe FinalizedRecipe) string {
	var b strings.Builder
	b.WriteString(recipe.Title + "\n\n")

	b.WriteString("Ingredients\n")
	for _, ing := range recipe.Ingredients {
		b.WriteString("- " + ing + "\n")
	}

	b.WriteString("\nSteps\n")
	for i, step := range recipe.Steps {
		b.WriteString(strconv.Itoa(i+1) + ". " + step + "\n")
	}

	if recipe.Notes != "" {
		b.WriteString("\nNotes\n" + recipe.Notes + "\n")
	}

	return b.String()
}
