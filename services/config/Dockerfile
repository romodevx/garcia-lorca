# Canonical multi-stage Go service image for García Lorca. See
# services/gateway/Dockerfile for the full rationale. Build from REPO ROOT:
#   docker build -f services/config/Dockerfile -t config .

# ---- builder --------------------------------------------------------------
FROM golang:1.25.13-bookworm AS builder

ENV CGO_ENABLED=0 GOOS=linux

WORKDIR /src

COPY pkg/go.mod pkg/go.sum* ./pkg/
COPY gen/go/go.mod gen/go/go.sum* ./gen/go/
COPY services/config/go.mod services/config/go.sum* ./services/config/

# Build a workspace containing ONLY what this image needs, instead of copying
# the repo go.work. That file lists all fourteen modules while this build
# context has three, so every go command failed with "cannot load module
# services/x listed in go.work file". The old `go work sync || true` swallowed
# that error and the build died one step later at `go build`, with a less
# obvious message.
RUN go work init ./gen/go ./pkg ./services/config \
    && (cd services/config && go mod download)

COPY pkg ./pkg
COPY gen ./gen
COPY services/config ./services/config

RUN go build \
      -trimpath \
      -ldflags="-s -w" \
      -o /out/server \
      ./services/config/cmd

# ---- runtime --------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /
COPY --from=builder /out/server /server

USER nonroot:nonroot

ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/server"]
