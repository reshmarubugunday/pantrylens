# Build context must be the repo root (not app/), because app/go.mod has
# `replace pantrylens/core => ../core` -- both modules need to be present
# with that relative path intact for `go build` to resolve it.
#
#   docker build -t pantrylens .
#   gcloud run deploy pantrylens --source . --region us-central1 \
#     --set-env-vars GOOGLE_CLOUD_PROJECT=...,GOOGLE_CLOUD_LOCATION=us-central1,GOOGLE_GENAI_USE_ENTERPRISE=1 \
#     --allow-unauthenticated
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
# ADK's web launcher already binds :8080 on all interfaces by default
# (google.golang.org/adk/v2/cmd/launcher/web), which matches Cloud Run's
# default expected port -- no extra flags needed.
ENTRYPOINT ["/pantrylens", "web"]
