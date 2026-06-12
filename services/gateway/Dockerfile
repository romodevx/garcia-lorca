# Canonical multi-stage Go service image for García Lorca.
#
# Build context is the REPO ROOT (not services/gateway), because each service is
# its own Go module wired through go.work and depends on ./pkg and ./gen/go.
# Build from root:
#   docker build -f services/gateway/Dockerfile -t gateway .
#
# Final stage is distroless static + nonroot: no shell, no package manager,
# minimal attack surface. CGO disabled, symbols stripped.

# ---- builder --------------------------------------------------------------
FROM golang:1.25-bookworm AS builder

ENV CGO_ENABLED=0 GOOS=linux

WORKDIR /src

# Workspace + module manifests first for layer-cached dependency download.
COPY go.work go.work.sum* ./
COPY pkg/go.mod pkg/go.sum* ./pkg/
COPY gen/go/go.mod gen/go/go.sum* ./gen/go/
COPY services/gateway/go.mod services/gateway/go.sum* ./services/gateway/
RUN go work sync || true

# Now the sources.
COPY pkg ./pkg
COPY gen ./gen
COPY services/gateway ./services/gateway

# Build the static binary.
RUN go build \
      -trimpath \
      -ldflags="-s -w" \
      -o /out/server \
      ./services/gateway/cmd

# ---- runtime --------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /
COPY --from=builder /out/server /server

# Distroless nonroot runs as uid 65532.
USER nonroot:nonroot

# Cloud Run injects PORT; the server must listen on it.
ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/server"]
