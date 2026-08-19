# pantrylens

PantryLens turns your fridge ingredients into recipes that fit your dietary rules, refined through conversation, then exported ready-to-cook.

## Go port

This mirrors the Python scaffold, ported to Go on top of the official
`google/adk-go` (module `google.golang.org/adk/v2`), which is GA and has
full parity with the Python SDK for tools, multi-turn agents, and Gemini.

## Two modules, on purpose

```
core/     module: pantrylens/core   -- zero external dependencies
app/      module: pantrylens/app    -- imports google.golang.org/adk/v2, genai
```

`core` holds the dietary-lens model and the deterministic compliance-check
logic — the same split as the Python version's `tools.py`, just made
structural here instead of a comment. Because it imports nothing but the Go
standard library, **it's the one part of this port that's actually verified
in the environment PantryLens was built in**: `gofmt`, `go vet`, and
`go test` all ran clean here, 8/8 tests passing (ported line-for-line from
the Python test suite, plus the same substring-vs-token-overlap fix that
bug caught originally).

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

`ModelName` in `app/agent.go` is `"gemini-2.0-flash"` — a placeholder, same
caveat as the Python version. **The hackathon requires Gemini 3.5 Flash or
newer.** Check the current model ID in the Model Garden / Gemini API docs
and update it before you submit.

## A known simplification, called out on purpose

Same as the Python version: `core.CheckRecipeAgainstLens`'s `mentions()`
helper matches on literal words/token-overlap, not food-category knowledge.
An avoid-entry of `"citrus"` won't infer that `"orange juice"` is a citrus
product. See the doc comment above `mentions()` in `core/tools.go` for the
full explanation — it's a fine thing to mention in the submission's
"learnings" section.

## Next step in the plan

Same Day 3/4 items as before — deepen the multi-turn refinement loop, and
swap `core.Registry`'s in-memory maps for Firestore-backed storage. Because
`Registry`'s methods are the only thing `tools_adk.go` calls into, that
swap stays contained to `core/tools.go`.
