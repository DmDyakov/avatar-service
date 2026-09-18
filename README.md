# Avatar Service

Микросервис для управления аватарками пользователей.

## Стек

- **Go 1.27**
- **PostgreSQL 16** — метаданные
- **MinIO (S3)** — файлы
- **chi** — HTTP router
- **zap** — логирование
- **golang-migrate** — миграции

## Быстрый старт

```bash
make infra-up        # PostgreSQL + MinIO
cp .env.example .env
make run             # сервер на :8080
```

## API

| Метод | Endpoint | Описание |
|-------|----------|----------|
| `POST` | `/api/v1/avatars` | Загрузить аватарку |
| `GET` | `/api/v1/avatars/{id}` | Получить файл |
| `GET` | `/api/v1/avatars/{id}/metadata` | Метаданные |
| `DELETE` | `/api/v1/avatars/{id}` | Удалить |
| `GET` | `/live` | Liveness |
| `GET` | `/ready` | Readiness (зависимости) |

### Пример: загрузка

```bash
curl -X POST http://localhost:8080/api/v1/avatars \
  -H "X-User-ID: user-123" \
  -F "file=@avatar.jpg"
```

```json
{
  "id": "84fac931-...",
  "user_id": "user-123",
  "file_name": "avatar.jpg",
  "mime_type": "image/jpeg",
  "size_bytes": 42379,
  "s3_key": "avatars/user-123/84fac931-...",
  "upload_status": "uploaded",
  "created_at": "2026-09-18T12:00:00Z"
}
```

### Пример: readiness

```bash
curl http://localhost:8080/ready
```

```json
{
  "status": "ok",
  "components": {
    "postgres": "ok",
    "storage": "ok"
  }
}
```

## Конфигурация

См. `.env.example`.

| Переменная | По умолчанию |
|-----------|-------------|
| `APP_ENV` | `dev` |
| `HTTP_ADDRESS` | `:8080` |
| `POSTGRES_DSN` | — |
| `S3_ENDPOINT` | `localhost:9000` |
| `S3_BUCKET` | `avatars` |

## Структура

```
cmd/                — точки входа (server, worker)
internal/
├── app/            — сборка зависимостей
├── config/         — конфигурация
├── domain/         — сущности и контракты
├── repository/     — PostgreSQL
├── services/       — бизнес-логика (avatar, health)
├── storage/        — MinIO
└── transport/      — HTTP
migrations/         — SQL-миграции
tests/              — E2E-тесты и фикстуры
```

## Команды

См. `Makefile`.

## Лицензия

MIT
