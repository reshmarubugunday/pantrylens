# PantryLens

**Track:** Collaborative Partner
**Tech:** Gemini 3.5 Flash · Google ADK Go SDK (`google.golang.org/adk/v2`) · Vertex AI · Firestore · Google Docs API · Cloud Run

## What it does

Give PantryLens a list of ingredients you have on hand and a "dietary lens" —
a configurable bundle of rules (allergens to avoid, macro targets,
health-condition constraints, or anything else) — and it proposes recipes
that fit, refined conversationally ("more protein," "no dairy," "use up the
spinach"). Once you're happy with one, it can export it to a real, shareable
Google Doc.

## The core idea: architectural discipline, not model trust

The differentiator isn't the recipe generation itself — it's what happens
before a recipe is ever shown to you. Every candidate recipe is run through
`check_recipe_against_lens`, a deterministic Go function (not another model
call), before the agent is allowed to present it. If a recipe violates the
active lens, the agent has to revise and re-check — it can't just show you
something and hope it's right. That check lives in `core/`, a dependency-free
Go package with its own full test suite, separate from all the ADK/Gemini
wiring in `app/`. The split is deliberate: the part of the system that
decides whether a recipe is safe to show you doesn't depend on the model
behaving correctly, and can be tested and trusted independently of it.

Two built-in lens presets ship to prove the system is generic, not tuned to
one person's diet: a GERD + hormonal-balance + macro-target lens, and an
unrelated Athletic Performance lens. You can also describe your own goals in
plain language and the agent will save them as a reusable custom lens.

## Tech stack

- **Gemini 3.5 Flash** via Vertex AI (not a plain AI Studio key — routed
  through a real GCP project so usage draws against GCP credit).
- **Google ADK Go SDK** (`google.golang.org/adk/v2`) for the agent, tool
  wiring, and conversation/session handling.
- **Firestore** as an opt-in backend for custom lens storage, so lenses
  survive process restarts (e.g. Cloud Run cold starts) instead of living
  only in memory.
- **Google Docs API** for the "real action" piece: turning a finalized,
  already-validated recipe into an actual, shareable document rather than
  leaving it as chat text.
- **Cloud Run** for deployment.

## Learnings and honest limitations

- The compliance checker's ingredient matcher (`mentions()` in
  `core/tools.go`) works on literal word/token overlap, not food-category
  knowledge. An avoid-rule for `"citrus"` won't infer that `"orange juice"`
  is a citrus product — that would need a synonym/category table or a
  second model call, which we scoped out deliberately. The agent's own
  instructions are the actual first line of defense on category-level
  ingredient judgment; the deterministic checker is a safety net on top of
  that, not a substitute for it. We think this honest boundary is more
  useful to state plainly than to paper over.
- Docs export currently assumes it's running under your own user
  credentials (local `console`/`web`), not a Cloud Run service account —
  a doc created by a service account is invisible to a human without an
  extra Drive-sharing step we didn't build under deadline. A production
  version would add that.

## Repo

github.com/reshmarubugunday/pantrylens
