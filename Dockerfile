# ---- Build stage ----
FROM golang:1.26-alpine AS build
WORKDIR /src

# cache de módulos
COPY go.mod go.sum ./
RUN go mod download

# build del binario estático
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/persister ./cmd/server

# ---- Runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/persister /persister
COPY docs/ /docs/
EXPOSE 8083
USER nonroot:nonroot
ENTRYPOINT ["/persister"]