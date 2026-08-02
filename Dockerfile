# ==============================================================================
# Multi-stage Dockerfile for Codebase-to-Docs Generator (codedocs)
# ==============================================================================

# --- Stage 1: Build stage ---
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install ca-certificates and git for dependency downloading
RUN apk add --no-cache ca-certificates git

# Download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source files
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o codedocs ./cmd/codedocs

# --- Stage 2: Minimal runtime stage ---
FROM alpine:latest

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/codedocs /app/codedocs

# Create temp and cache directories
RUN mkdir -p /app/temp_docs /root/.cache/codedocs

EXPOSE 8080

ENTRYPOINT ["/app/codedocs"]
CMD ["--port", "8080", "--host", "0.0.0.0"]
