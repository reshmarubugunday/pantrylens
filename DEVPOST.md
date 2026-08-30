# PantryLens

**Track:** Collaborative Partner
**Tech:** Gemini 3.5 Flash · Google ADK Go SDK (`google.golang.org/adk/v2`) · Vertex AI · Firestore · Cloud Run
**Live demo:** https://pantrylens-765410575676.us-central1.run.app
**Architecture diagram:** see README.md ("Architecture" section, top of file)

## What it does

Give PantryLens a list of ingredients you have on hand (typed, pasted, or
detected from a photo) and one or more "dietary lenses" — configurable
bundles of rules (allergens to avoid, macro targets, health-condition
constraints, or anything else), stackable so e.g. Vegetarian + GERD-Friendly
both apply to the same recipe at once — and it proposes recipes that fit,
refined conversationally ("more protein," "no dairy," "use up the
spinach"). Ask for one meal or a whole week of meal prep (a batch of
recipes that deliberately share ingredients, each with its own storage
note); you choose the meal type, the agent never assumes dinner. A session
survives a page refresh, and every saved recipe gets its own shareable page,
filterable by meal type in "My saved recipes."

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

Thirteen built-in lens presets ship to prove the system is generic, not
tuned to one person's diet: GERD-Friendly, Hormonal Balance, Macro Target,
Athletic Performance, Vegetarian, Vegan, Diabetic-Friendly, Heart-Healthy,
Gluten-Free, Dairy-Free, Keto, Low-FODMAP, and Kidney-Friendly (GERD,
hormonal balance, and macro targets are deliberately three separate,
combinable lenses, not one bundle -- someone might want any one of them
without the other two). Any number can be active together --
`CombineLenses` in `core/tools.go` merges their avoid/prefer lists, macro
targets, and rules into one set a recipe has to satisfy all at once, not
just whichever lens happened to get checked. You can also describe your own
goals in plain language, on top of any presets, and the agent will save
them as a reusable custom lens.

## Tech stack

- **Gemini 3.5 Flash** via Vertex AI (not a plain AI Studio key — routed
  through a real GCP project so usage draws against GCP credit).
- **Google ADK Go SDK** (`google.golang.org/adk/v2`) for the agent, tool
  wiring, and conversation/session handling.
- **Firestore** as an opt-in backend for custom lenses, saved recipes, and
  per-user preferences, so all three survive process restarts (e.g. Cloud
  Run cold starts) instead of living only in memory -- each falls back to
  in-memory automatically if Firestore is unreachable. Saving a recipe is
  the "real action" piece: a finalized, already-validated recipe becomes
  its own standalone, shareable page rather than staying chat text.
- **Gemini vision** (via `google.golang.org/genai`, one-shot call, JSON
  schema-constrained output) for photo-based ingredient detection.
- **Cloud Run** for deployment.
- A small, self-contained **web UI** (plain HTML/CSS/JS, no build step)
  talking to ADK's REST API, in place of ADK's built-in developer console.

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
- A saved recipe's link is unguessable (a random 128-bit ID) but not
  access-controlled or expiring -- anyone with the link can view it,
  forever. Fine for a hackathon demo; a production version would add
  expiry and/or scope links to the user who saved them.

## Repo

github.com/reshmarubugunday/pantrylens
