# Canonical multi-stage Go service image for García Lorca. See
# services/gateway/Dockerfile for the full rationale. Build from REPO ROOT:
#   docker build -f services/finance/Dockerfile -t finance .

# ---- builder --------------------------------------------------------------
FROM golang:1.25-bookworm AS builder

ENV CGO_ENABLED=0 GOOS=linux

WORKDIR /src

COPY go.work go.work.sum* ./
COPY pkg/go.mod pkg/go.sum* ./pkg/
COPY gen/go/go.mod gen/go/go.sum* ./gen/go/
COPY services/finance/go.mod services/finance/go.sum* ./services/finance/
RUN go work sync || true

COPY pkg ./pkg
COPY gen ./gen
COPY services/finance ./services/finance

RUN go build \
      -trimpath \
      -ldflags="-s -w" \
      -o /out/server \
      ./services/finance/cmd

# ---- runtime --------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /
COPY --from=builder /out/server /server

USER nonroot:nonroot

ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/server"]
