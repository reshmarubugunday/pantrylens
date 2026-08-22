# pantrylens

PantryLens turns your fridge ingredients into recipes that fit your dietary rules, refined through conversation, then exported ready-to-cook.

## Go port

This mirrors the Python scaffold, ported to Go on top of the official
`google/adk-go` (module `google.golang.org/adk/v2`), which is GA and has
full parity with the Python SDK for tools, multi-turn agents, and Gemini.

## Two modules, on purpose

```
core/     module: pantrylens/core   -- lens model + compliance logic, no external deps;
                                        Firestore is an opt-in storage backend (see below)
app/      module: pantrylens/app    -- imports google.golang.org/adk/v2, genai
```

`core` holds the dietary-lens model and the deterministic compliance-check
logic — the same split as the Python version's `tools.py`, just made
structural here instead of a comment. The lens model, the compliance
checker, and their tests (`core/tools_test.go`) import nothing but the Go
standard library: `gofmt`, `go vet`, and `go test` all run clean with zero
setup, 8/8 tests passing. The one exception is `core/firestore_store.go`,
an opt-in `LensStore` implementation pulled in only if you use
`core.NewFirestoreRegistry` (see "Firestore-backed lens storage" below) --
it's the only file in the module that imports anything beyond stdlib, and
the existing tests don't exercise it.

`app` is the ADK wiring — the agent, the tool adapters, the CLI entrypoint.
It's written directly against the real, current ADK Go API (I read the
actual `google/adk-go` source — `tool/functiontool/function.go`,
`examples/tools/multipletools/main.go`, `agent/llmagent/llmagent.go`,
`runner/runner.go` — rather than guessing at method names). **It could not
be compiled in that environment**: `google.golang.org/adk/v2` requires Go
1.26+ (that environment had 1.24.7) and fetching the module needs open
network access to `proxy.golang.org` / `google.golang.org` (that
environment's egress is allowlisted to a smaller set of hosts and blocked
both). `gofmt` confirms every file in `app/` is syntactically valid Go;
what's unverified is that the ADK API calls type-check exactly as written
against the live dependency versions. Run the setup below on your own
machine (or Cloud Shell) to find out, and treat the first `go build` as a
real check, not a formality.

## Setup

**1. Install Go 1.26+.** Check with `go version` — if you're on an older
version, grab the latest from https://go.dev/dl/.

**2. Resolve dependencies** (this is the step that couldn't run in the
scaffolding environment):

```bash
cd app
go mod tidy
```

This pulls in `google.golang.org/adk/v2` and `google.golang.org/genai` at
their current versions and rewrites `go.mod`/`go.sum` accordingly — the
version numbers in `app/go.mod` right now are placeholders, don't rely on
them.

**3. Build and vet:**

```bash
go build ./...
go vet ./...
```

If something doesn't type-check, it's most likely one of two things: the
`jsonschema` struct tag usage in `tools_adk.go` (I confirmed the tag syntax
— bare description text, e.g. `` `jsonschema:"lens name"` `` — against the
`google/jsonschema-go` docs, but haven't run it against a real schema
build), or a signature drift in `functiontool.New` / `llmagent.New` /
`gemini.NewModel` if the library shipped a breaking change after this was
written. Both are quick fixes if you hit them — the shapes are right even
if a detail needs adjusting.

## Using your $300 GCP credit

Same gotcha as the Python version: a plain AI Studio API key does **not**
draw against a GCP trial credit — you need Vertex AI inside a real GCP
project with billing attached. Full walkthrough (project creation, billing,
`gcloud` setup, enabling APIs, Firestore) is in the Python scaffold's
README; the short version for this Go port:

```bash
gcloud auth application-default login
gcloud config set project YOUR_PROJECT_ID
gcloud services enable aiplatform.googleapis.com firestore.googleapis.com run.googleapis.com docs.googleapis.com

export GOOGLE_GENAI_USE_ENTERPRISE=1
export GOOGLE_CLOUD_PROJECT=your-real-project-id
export GOOGLE_CLOUD_LOCATION=us-central1
```

`gemini.NewModel` in `app/agent.go` is called with an empty
`&genai.ClientConfig{}`, which resolves backend/project/location from
those env vars automatically — no API key needed in this mode.

## Run it

```bash
cd app/cmd/pantrylens
go run . console   # chat with the agent in your terminal
go run . web       # local web UI, prints a URL to open
```

## Run the core tests (works right now, no setup needed)

```bash
cd core
go test ./... -v
```

## Important: verify the model name before you submit

`ModelName` in `app/agent.go` is now `"gemini-3.5-flash"` — matching the
pinned `google.golang.org/genai` SDK's own current example code, not a
placeholder anymore. **The hackathon requires Gemini 3.5 Flash or newer.**
Model availability can still vary by GCP project/region, so run the agent
once against your own project (`go run . console`) and confirm you get a
real response, not a model-not-found error, before you submit. If it's
unavailable, check Model Garden for the closest current name.

## A known simplification, called out on purpose

Same as the Python version: `core.CheckRecipeAgainstLens`'s `mentions()`
helper matches on literal words/token-overlap, not food-category knowledge.
An avoid-entry of `"citrus"` won't infer that `"orange juice"` is a citrus
product. See the doc comment above `mentions()` in `core/tools.go` for the
full explanation — it's a fine thing to mention in the submission's
"learnings" section.

## Firestore-backed lens storage (optional)

By default the agent uses an in-memory lens registry (`core.NewRegistry`) --
custom lenses created via `save_custom_lens` live only for the process's
lifetime. Set `GOOGLE_CLOUD_PROJECT` and the agent will instead use
`core.NewFirestoreRegistry` (collection `dietary_lenses`), so custom lenses
survive restarts -- useful once you're running on Cloud Run, where a cold
start would otherwise lose them. If Firestore construction fails for any
reason (auth, network), it logs a warning and falls back to in-memory, so
local/offline dev keeps working unchanged. Built-in presets always work
either way -- they come from `BuiltInLenses()`, not the store.

## Exporting a recipe to Google Docs

Once the user is happy with a validated recipe, the agent can call
`export_recipe_to_doc` (see `app/export_docs.go`) to create a real, shareable
Google Doc via the Docs API. This is demoed against your own
`gcloud auth application-default login` user credentials (local `console`/
`web`); a doc created under a Cloud Run service account would be owned by
that service account and invisible to a human without an extra Drive-sharing
step this scaffold doesn't implement, so don't rely on this feature through
a Cloud Run deployment. If you hit a 403 calling it, your ADC likely needs
broader scopes:

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/cloud-platform,https://www.googleapis.com/auth/documents,https://www.googleapis.com/auth/drive.file
```

## Deploy to Cloud Run

The `web` launcher already binds `:8080` on all interfaces by default, which
matches what Cloud Run expects, so no code changes are needed:

```bash
gcloud run deploy pantrylens --source . --region us-central1 \
  --set-env-vars GOOGLE_CLOUD_PROJECT=your-project-id,GOOGLE_CLOUD_LOCATION=us-central1,GOOGLE_GENAI_USE_ENTERPRISE=1 \
  --allow-unauthenticated

gcloud projects add-iam-policy-binding your-project-id \
  --member="serviceAccount:<cloud-run-service-account>" --role="roles/aiplatform.user"
# If using Firestore-backed lens storage too:
gcloud projects add-iam-policy-binding your-project-id \
  --member="serviceAccount:<cloud-run-service-account>" --role="roles/datastore.user"
```

Demo the core ingredients → lens → recipe loop through the deployed URL;
keep the Docs export feature demoed locally (see above).

## Next step in the plan

See the hackathon completion plan for the remaining prioritized work
(live model-name verification, scripted multi-turn QA, and Devpost
submission prep) -- the core loop, Docs export, Firestore-backed storage,
and Cloud Run deployment described above are implemented.
