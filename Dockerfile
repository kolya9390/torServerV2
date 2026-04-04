# TorrServer v3.0 — Минималистичный Dockerfile
# Без Web UI, без Prometheus, без лишнего

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

# Установка зависимостей для сборки
RUN apk add --update git

WORKDIR /src

# Кэширование зависимостей Go
COPY server/go.mod server/go.sum ./
RUN go mod download

# Копирование исходников
COPY server/ ./

# Сборка с автоматическим определением платформы
ARG TARGETOS TARGETARCH
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH CGO_ENABLED=0 go build -ldflags '-w -s' -o /torrserver ./cmd


### runtime image
FROM alpine:3.21

# Установка минимальных зависимостей
RUN apk add --no-cache ffmpeg ca-certificates tzdata tini

# Копирование бинарника
COPY --from=builder /torrserver /usr/bin/torrserver

# Создание директорий
RUN mkdir -p /opt/ts/config /opt/ts/torrents /opt/ts/log

# Порты
# 8090 — HTTP API
# 9080 — DLNA
EXPOSE 8090 9080

# Переменные окружения по умолчанию
ENV TS_PORT=8090
ENV TS_DLN=1
ENV TS_CONF_PATH=/opt/ts/config
ENV TS_TORR_DIR=/opt/ts/torrents
ENV TS_LOG_PATH=/opt/ts/log

# Healthcheck
HEALTHCHECK --interval=60s --timeout=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8090/healthz || exit 1

# Точка входа с tini для корректной обработки сигналов
ENTRYPOINT ["/sbin/tini", "--", "/usr/bin/torrserver"]
