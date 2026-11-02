# Avatar Service

Микросервис для управления аватарками пользователей. Принимает изображения, хранит их в S3, генерирует миниатюры в фоне и отдаёт по REST API.

## Стек

- Go 1.21+ (сборка на 1.27)
- PostgreSQL 16 — метаданные
- MinIO (S3) — файлы
- RabbitMQ — очередь задач для воркера
- chi — HTTP-роутер
- zap — логирование
- golang-migrate — миграции
- OpenAPI 3.0 — спецификация API

## Быстрый старт

Скопируйте `.env.example` в `.env`:

```
cp .env.example .env
```

Дальше два варианта — для разработки и для проверки в Docker.

### Разработка (приложение на хосте)

Инфраструктура поднимается в Docker, приложение запускается локально. Так быстрее итерировать: не нужно пересобирать образ после каждой правки.

В трёх терминалах:

```
make infra-up    # postgres, minio, rabbitmq
make run         # HTTP-сервер на :8080
make worker      # воркер обработки изображений
```

Проверить, что сервер поднялся:

```
curl http://localhost:8080/live
curl http://localhost:8080/ready
```

Открыть форму загрузки в браузере:

```
http://localhost:8080/web/upload
```

Остановить инфраструктуру (данные сохраняются):

```
make infra-down
```

### Всё в Docker

Когда нужно проверить, что образ собирается и сервисы работают вместе:

```
docker compose up -d --build
curl http://localhost:8080/live
docker compose down
```

`docker compose down` останавливает контейнеры, но данные (Postgres, MinIO, RabbitMQ) сохраняются в volumes. Если нужно сбросить всё до чистого состояния — `docker compose down -v`. Внимание: `-v` удаляет данные безвозвратно.

Логи приложения в Docker:

```
docker compose logs -f server worker
```

## API

| Метод | Endpoint | Описание | Статус |
|-------|----------|----------|--------|
| POST | `/api/v1/avatars` | Загрузить аватарку | ✅ Sprint 1 |
| GET | `/api/v1/avatars/{id}` | Получить файл | ✅ Sprint 1 |
| GET | `/api/v1/users/{user_id}/avatar` | Текущая аватарка пользователя | ✅ Sprint 1 |
| GET | `/live` | Liveness | ✅ Sprint 1 |
| GET | `/ready` | Readiness (зависимости) | ✅ Sprint 1 |
| GET | `/health` | Алиас `/ready` | ✅ Sprint 1 |
| GET | `/web/upload` | HTML-форма загрузки | ✅ Sprint 1 |
| POST | `/web/upload` | Обработка формы (нативный submit) | ✅ Sprint 1 |
| GET | `/api/v1/avatars/{id}/metadata` | Метаданные | ⏳ Sprint 2 (AS-204) |
| GET | `/api/v1/users/{user_id}/avatars` | Все аватарки пользователя | ⏳ Sprint 2 (AS-205) |
| GET | `/web/gallery/{user_id}` | Галерея аватарок | ⏳ Sprint 2 (AS-206) |
| DELETE | `/api/v1/avatars/{id}` | Удалить | ⏳ Sprint 3 (AS-301) |

### Загрузка через API

```
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
  "width": 540,
  "height": 360,
  "upload_status": "uploaded",
  "processing_status": "pending",
  "attempts": 0,
  "created_at": "2026-09-18T12:00:00Z",
  "updated_at": "2026-09-18T12:00:00Z"
}
```

`width`/`height` заполняются при upload (`image.DecodeConfig`). `processing_status: pending` — задача на миниатюры в очереди (воркер — Sprint 2).

### Readiness

```
curl http://localhost:8080/ready
```

```json
{
  "status": "ok",
  "components": {
    "postgres": "ok",
    "storage": "ok",
    "rabbitmq": "ok"
  }
}
```

## OpenAPI

Спецификация всех эндпоинтов — в [`internal/api/openapi.yaml`](internal/api/openapi.yaml). Описывает контракты (методы, коды, схемы, заголовки), включая запланированные на Sprint 2–3 (`x-status: planned`).

**Проверить спеку:**

```
make openapi-validate
```

**Посмотреть в Swagger UI:**

```
make openapi-ui
# открыть http://localhost:8081
```

Спека не раздаётся API-сервером — это документ для команды и внешних потребителей. Раздача `GET /openapi.yaml` — Phase 2 (AS-1103).

## Конфигурация

Все переменные описаны в `.env.example`. Основные:

| Переменная | По умолчанию | Описание |
|-----------|-------------|----------|
| `APP_ENV` | `dev` | `dev`, `staging` или `prod` |
| `HTTP_ADDRESS` | `:8080` | адрес HTTP-сервера |
| `POSTGRES_HOST` | `localhost` | хост Postgres (в Docker — `postgres`) |
| `POSTGRES_PORT` | `5432` | порт Postgres |
| `S3_ENDPOINT` | `localhost:9000` | адрес S3 |
| `S3_BUCKET` | `avatars` | бакет для файлов |
| `S3_FORCE_PATH_STYLE` | `false` | `true` для MinIO, `false` для AWS |
| `RABBITMQ_HOST` | `localhost` | хост RabbitMQ (в Docker — `rabbitmq`) |
| `RABBITMQ_PREFETCH` | `10` | сколько задач воркер держит одновременно |

DSN для Postgres и URL для RabbitMQ собираются в коде из отдельных полей (`config.PostgresConfig.DSN()` и `config.RabbitMQConfig.URL()`). Отдельных переменных `POSTGRES_DSN` и `RABBITMQ_URL` нет — это сделано, чтобы приложение и docker-compose использовали одни и те же значения без дублирования.

## Структура

```
cmd/                — точки входа (server, worker)
internal/
├── api/            — OpenAPI-спецификация
├── app/            — сборка зависимостей
├── broker/         — RabbitMQ (клиент, publisher, топология)
├── config/         — конфигурация
├── domain/         — сущности и контракты
├── logger/         — обёртка над zap
├── repository/     — PostgreSQL (пул, миграции, репозитории)
├── services/       — бизнес-логика
├── storage/        — MinIO (S3)
└── transport/      — HTTP (сервер, хендлеры, middleware)
migrations/         — SQL-миграции (встроены в бинарник)
web/                — веб-интерфейс (HTML-форма)
tests/              — E2E-тесты и фикстуры
```

## Команды

```
make help              # список всех команд
make run               # сервер
make worker            # воркер
make test              # тесты
make lint              # golangci-lint
make infra-up          # postgres + minio + rabbitmq
make infra-down        # остановить инфраструктуру
make docker-up         # всё в Docker
make docker-down       # остановить Docker
make openapi-validate  # проверить OpenAPI-спеку
make openapi-ui        # Swagger UI на http://localhost:8081
```

## Лицензия

MIT
