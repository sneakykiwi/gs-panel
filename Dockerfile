FROM golang:1.25.6-alpine AS builder

RUN apk add --no-cache git gcc musl-dev sqlite-dev

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=0.0.1
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-w -s \
    -X gs-panel/internal/version.Version='${VERSION}' \
    -X gs-panel/internal/version.GitCommit='${GIT_COMMIT}' \
    -X gs-panel/internal/version.BuildTime='${BUILD_TIME}'" \
    -o gs-panel \
    cmd/server/main.go

FROM alpine:latest

RUN apk add --no-cache ca-certificates sqlite-libs

RUN adduser -D -h /data gs-panel

WORKDIR /data

COPY --from=builder /build/gs-panel /usr/local/bin/gs-panel

COPY --from=builder /build/web /data/web

RUN mkdir -p /data/servers /data/backups /data/logs && \
    chown -R gs-panel:gs-panel /data

USER gs-panel

EXPOSE 8080

VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/ || exit 1

ENTRYPOINT ["gs-panel"]
