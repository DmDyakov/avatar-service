# ---------- Build stage ----------
FROM golang:1.27-alpine AS builder

WORKDIR /app

# Сначала go.mod/go.sum — кэш зависимостей переиспользуется, пока не изменится код
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Версия сборки - прокидывается из docker compose build.args
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
  -ldflags "-X avatar-service/pkg/buildinfo.Version=${VERSION} \
  -X avatar-service/pkg/buildinfo.Commit=${COMMIT} \
  -X avatar-service/pkg/buildinfo.Date=${BUILD_DATE}" \
  -o /bin/server ./cmd/server

RUN CGO_ENABLED=0 GOOS=linux go build \
  -ldflags "-X avatar-service/pkg/buildinfo.Version=${VERSION} \
  -X avatar-service/pkg/buildinfo.Commit=${COMMIT} \
  -X avatar-service/pkg/buildinfo.Date=${BUILD_DATE}" \
  -o /bin/worker ./cmd/worker

# ---------- Runtime stage ----------
FROM alpine:3.20

RUN apk --no-cache add ca-certificates tzdata \
  && addgroup -S app \
  && adduser -S app -G app

WORKDIR /app

COPY --from=builder --chown=app:app /bin/server /bin/worker ./
COPY --from=builder --chown=app:app /app/web ./web/

USER app

EXPOSE 8080

CMD ["./server"]
