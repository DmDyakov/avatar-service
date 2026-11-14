# Changelog

Все значимые изменения в проекте. Формат — [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/),
версионирование — [SemVer](https://semver.org/lang/ru/).

## [Unreleased]

## [0.1.0-alpha.1] - 2026-11-13

Первый alpha-релиз MVP. Все эндпоинты из ТЗ реализованы.

### Added

**API**
- `POST /api/v1/avatars` — загрузка (multipart, JPEG/PNG/WebP, до 10 MB)
- `GET /api/v1/avatars/{id}` — файл по ID (`Content-Type`, `Cache-Control`)
- `GET /api/v1/users/{user_id}/avatar` — текущая аватарка пользователя
- `GET /api/v1/avatars/{id}/metadata` — метаданные (`dimensions`, `thumbnails`)
- `GET /api/v1/users/{user_id}/avatars` — список аватарок пользователя
- `DELETE /api/v1/avatars/{id}` — soft delete + async очистка S3
- `DELETE /api/v1/users/{user_id}/avatar` — удаление текущей аватарки

**Веб-интерфейс**
- `GET /web/upload` — форма загрузки с превью (серверный `<img>` после 201)
- `POST /web/upload` — нативная обработка формы (PRG, работает без JS)
- `GET /web/gallery/{user_id}` — галерея с сеткой, кнопкой удаления и empty state

**Асинхронная обработка**
- Генерация миниатюр 100×100 и 300×300 (worker, RabbitMQ)
- Очистка S3 при удалении аватарки (worker, отдельная очередь)
- Retry с exponential backoff (2s → 4s → 8s) и DLX/DLQ

**Инфраструктура**
- Health-проверки: `/live`, `/ready`, `/health`
- Rate limiting: per-IP, token bucket, отдельный лимит на upload
- CORS для `/api/v1` (preflight, конкретные origins из env)
- OpenAPI 3.0 спецификация (`internal/api/openapi.yaml`) + Swagger UI
- Миграции БД через `golang-migrate` (встроены в бинарник через `go:embed`)
- Docker multi-stage build, non-root, ~25 MB образ

**Качество**
- Unit и интеграционные тесты (`testcontainers-go`: PG, MinIO, RabbitMQ)
- `golangci-lint` v2, `make check` (lint + coverage + openapi)
- Покрытие по бизнес-пакетам ≥50%

### Security
- Валидация по magic bytes (JPEG/PNG/WebP), не по `Content-Type`
- `http.MaxBytesReader` (защита от DoS через большие тела)
- Валидация `X-User-ID` (regex `^[a-zA-Z0-9_\-]+$`, длина ≤255)
- Секреты (`PASSWORD`, `ACCESS_KEY`, `SECRET_KEY`) — `required`, без дефолтов
- Маскирование паролей в логах (`DSNRedacted`, `URLRedacted`)
