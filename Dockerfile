# Build stage
FROM golang:1.25.6-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev sqlite-dev

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Get version from git or use default
ARG VERSION=0.0.1
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

# Build the binary with version injection
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-w -s \
    -X gs-panel/internal/version.Version=${VERSION} \
    -X gs-panel/internal/version.GitCommit=${GIT_COMMIT} \
    -X gs-panel/internal/version.BuildTime=${BUILD_TIME}" \
    -o gs-panel \
    cmd/server/main.go

# Runtime stage
FROM alpine:latest

# Install runtime dependencies only
RUN apk add --no-cache ca-certificates sqlite-libs

# Create non-root user
RUN adduser -D -h /data gs-panel

WORKDIR /data

# Copy binary from builder
COPY --from=builder /build/gs-panel /usr/local/bin/gs-panel

# Copy web assets
COPY --from=builder /build/web /data/web

# Create directories
RUN mkdir -p /data/servers /data/backups /data/logs && \
    chown -R gs-panel:gs-panel /data

# Switch to non-root user
USER gs-panel

# Expose port
EXPOSE 8080

# Volume for persistent data
VOLUME ["/data"]

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/ || exit 1

# Run the panel
ENTRYPOINT ["gs-panel"]
