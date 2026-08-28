# Build context must be the repo root (not app/), because app/go.mod has
# `replace pantrylens/core => ../core` -- both modules need to be present
# with that relative path intact for `go build` to resolve it.
#
#   docker build -t pantrylens .
#   gcloud run deploy pantrylens --source . --region us-central1 \
#     --set-env-vars GOOGLE_CLOUD_PROJECT=...,GOOGLE_CLOUD_LOCATION=global,GOOGLE_GENAI_USE_ENTERPRISE=1 \
#     --allow-unauthenticated
#
# (GOOGLE_CLOUD_LOCATION must be "global", not a region -- confirmed live,
# gemini-3.5-flash 404s as a Vertex publisher model on regional endpoints.)
#
# (verify the golang:1.26 base image tag is published by the time you
# build this -- app/go.mod requires go 1.26; golang:1.26-rc or a pinned
# patch version are the fallbacks if it isn't yet.)

FROM golang:1.26 AS build
WORKDIR /src
COPY core/ core/
COPY app/ app/
WORKDIR /src/app
RUN CGO_ENABLED=0 go build -o /pantrylens ./cmd/pantrylens

FROM gcr.io/distroless/base-debian12
COPY --from=build /pantrylens /pantrylens
ENV GOOGLE_GENAI_USE_ENTERPRISE=1
# ADK's web launcher binds :8080 on all interfaces by default
# (google.golang.org/adk/v2/cmd/launcher/web), which matches Cloud Run's
# default expected port. "web" alone starts no sub-servers, though -- ui
# (PantryLens's own frontend, see app/cmd/pantrylens/frontend) and api (the
# REST backend it talks to) must be named explicitly.
#
# -write-timeout/-read-timeout raise ADK's 15s default (confirmed live: a
# multi-recipe turn -- draft, check_recipe_against_lens, propose_recipe per
# candidate -- routinely takes 20-40s, and meal-prep batches longer still;
# at the 15s default the connection gets killed right as the response is
# ready, surfacing to the browser as a bare "failed to fetch"). These flags
# are sublauncher-config, so they must come before "ui api", not after.
ENTRYPOINT ["/pantrylens", "web", "-write-timeout=180s", "-read-timeout=180s", "ui", "api"]
