# TorrServer — Minimal multi-stage Dockerfile
# No Web UI, no Prometheus, production-ready

### Build stage
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /src

# Cache Go modules (copy manifests first)
COPY server/go.mod server/go.sum ./
RUN go mod download

# Copy source code and build
COPY server/ ./

ARG TARGETOS TARGETARCH
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH CGO_ENABLED=0 \
    go build -ldflags '-w -s' -o /torrserver ./cmd


### Runtime stage
FROM alpine:3.21

# Install runtime dependencies only
RUN apk add --no-cache ca-certificates tini tzdata

# Create non-root user
RUN addgroup -g 1000 torrserver && \
    adduser -u 1000 -G torrserver -s /bin/sh -D torrserver

# Copy binary from builder
COPY --from=builder /torrserver /usr/bin/torrserver

# Create data directories
RUN mkdir -p /opt/ts/config /opt/ts/torrents /opt/ts/log && \
    chown -R torrserver:torrserver /opt/ts

# Expose ports
# 8090 — HTTP API
# 9080 — DLNA
EXPOSE 8090 9080

# Default environment variables
ENV TS_PORT=8090
ENV TS_DLN=1
ENV TS_CONF_PATH=/opt/ts/config
ENV TS_TORR_DIR=/opt/ts/torrents
ENV TS_LOG_PATH=/opt/ts/log

# Health check
HEALTHCHECK --interval=60s --timeout=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8090/healthz || exit 1

# Run as non-root user with tini for signal handling
USER torrserver
ENTRYPOINT ["/sbin/tini", "--", "/usr/bin/torrserver"]
