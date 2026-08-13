FROM golang:1.26.4-alpine AS builder

ARG VERSION=dev
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w -X codedocs/internal/config.Version=${VERSION}" \
    -o /out/codedocs ./cmd/codedocs

FROM alpine:3.23

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S codedocs \
    && adduser -S -G codedocs -h /data codedocs \
    && mkdir -p /data/output /data/cache /data/config \
    && chown -R codedocs:codedocs /data

COPY --from=builder /out/codedocs /usr/local/bin/codedocs

USER codedocs
WORKDIR /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/api/ping || exit 1

ENTRYPOINT ["/usr/local/bin/codedocs"]
CMD ["--port", "8080", "--host", "0.0.0.0", "--open-browser=false", "--temp-dir", "/data/output", "--cache-dir", "/data/cache", "--bookmark-file", "/data/config/saved_paths.json"]
