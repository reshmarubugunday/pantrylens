package core

import "context"

// IngredientDetector identifies edible ingredients from a photo (e.g. of a
// fridge or pantry) -- a one-shot classification call, deliberately
// separate from LensStore/PreferenceStore since it has no persistence of
// its own and nothing to do with a particular user. Implemented in
// package app (see ../app/vision.go) using Gemini's vision capability,
// since actually calling a model is beyond what this dependency-free
// package takes on -- see the package doc comment on why core stays
// stdlib-only.
type IngredientDetector interface {
	// Detect returns the short, generic ingredient names it can identify
	// in the given image. imageBytes is the raw, undecoded image file
	// contents; mimeType is its content type (e.g. "image/jpeg").
	Detect(ctx context.Context, imageBytes []byte, mimeType string) ([]string, error)
}
