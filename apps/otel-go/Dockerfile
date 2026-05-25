# Multi-stage build — final image is distroless (no shell, no package manager).
# Build: docker build -t ghcr.io/<user>/otel-go:latest .
# Run:   docker run --rm -p 8080:8080 -p 2112:2112 \
#          -e OTEL_TRACES_EXPORTER=console \
#          -e OTEL_METRICS_EXPORTER=console \
#          -e OTEL_LOGS_EXPORTER=console \
#          ghcr.io/<user>/otel-go:latest

FROM docker.io/library/golang:1.26-alpine AS build
WORKDIR /src

# Download dependencies in a separate layer for cache efficiency.
COPY go.mod go.sum ./
RUN go mod download

COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /otel-go .

# Distroless base — scratch-like but with CA certs and tzdata.
FROM gcr.io/distroless/static-debian12
COPY --from=build /otel-go /otel-go

EXPOSE 8080 2112

ENTRYPOINT ["/otel-go"]
