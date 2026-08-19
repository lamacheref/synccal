# Dockerfile for SyncCal
# Multi-stage build for minimal production image

# Build stage
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git make

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o synccal ./cmd/synccal

# Runtime stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata sqlite-libs

# Non-root user
RUN adduser -D -u 1000 synccal
USER synccal

WORKDIR /app

COPY --from=builder /app/synccal .
COPY --from=builder /app/config.example.yaml ./config.yaml

# Healthcheck
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:8080/healthz || exit 1

EXPOSE 8080

ENTRYPOINT ["./synccal"]