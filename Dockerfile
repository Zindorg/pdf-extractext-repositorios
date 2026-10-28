# syntax=docker/dockerfile:1

# Persistidor: Go estático + distroless (sin shell ni cliente HTTP) y sin root.
ARG GO_VERSION=1.27

# ---------- Build ----------
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

# Requiere .dockerignore (.git, .env, tests) para no copiar basura ni secretos.
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/persister ./cmd/server

# ---------- Runtime ----------
FROM gcr.io/distroless/static-debian12:nonroot

ARG VERSION=dev
LABEL org.opencontainers.image.title="pdf-extractext-persister" \
      org.opencontainers.image.description="Consumidor del stream de eventos y persistencia en MongoDB" \
      org.opencontainers.image.source="https://github.com/Zindorg/pdf-extractext-repositorios" \
      org.opencontainers.image.version="${VERSION}"

COPY --from=build --chmod=0555 /out/persister /persister
COPY --chmod=0444 docs/ /docs/

USER 65532:65532

EXPOSE 8083
ENTRYPOINT ["/persister"]