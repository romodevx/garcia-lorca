# syntax=docker/dockerfile:1
#
# Dev image: hot-reloading Go service runner (air). Source is MOUNTED at runtime
# (docker compose), not copied — so a single image serves every service, picked
# by the $SERVICE env var. NOT for production: prod uses the distroless
# multi-stage images under services/*/Dockerfile.
FROM golang:1.26-bookworm

# air — live reload for Go (github.com/air-verse/air).
# Pinned, NOT @latest: this image runs with GOTOOLCHAIN=local (no auto-upgrade),
# so an air release that needs a newer Go than the base image breaks the build.
# v1.66.0 builds on Go 1.26. Bump this in lockstep with the base image's Go version.
ARG AIR_VERSION=v1.66.0
RUN go install github.com/air-verse/air@${AIR_VERSION}

WORKDIR /workspace

COPY docker/dev-entrypoint.sh /usr/local/bin/dev-entrypoint
RUN chmod +x /usr/local/bin/dev-entrypoint

# Services listen on :8080 inside the network.
ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/dev-entrypoint"]
