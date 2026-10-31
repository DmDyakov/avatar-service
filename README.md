# Avatar Service

Микросервис для управления аватарками пользователей. Принимает изображения, хранит их в S3, генерирует миниатюры в фоне и отдаёт по REST API.

## Стек

- Go 1.27
- PostgreSQL 16 — метаданные
- MinIO (S3) — файлы
- RabbitMQ — очередь задач для воркера
- chi — HTTP-роутер
- zap — логирование
- golang-migrate — миграции

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

| Метод | Endpoint | Описание |
|-------|----------|----------|
| POST | `/api/v1/avatars` | Загрузить аватарку |
| GET | `/api/v1/avatars/{id}` | Получить файл |
| GET | `/api/v1/avatars/{id}/metadata` | Метаданные |
| DELETE | `/api/v1/avatars/{id}` | Удалить |
| GET | `/api/v1/users/{user_id}/avatar` | Текущая аватарка пользователя |
| GET | `/api/v1/users/{user_id}/avatars` | Все аватарки пользователя |
| GET | `/live` | Liveness |
| GET | `/ready` | Readiness (зависимости) |
| GET | `/web/upload` | Форма загрузки |
| GET | `/web/gallery/{user_id}` | Галерея |

Загрузка:

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
  "upload_status": "uploaded",
  "created_at": "2026-09-18T12:00:00Z"
}
```

Readiness:

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
| `RABBITMQ_HOST` | `localhost` | хост RabbitMQ (в Docker — `rabbitmq`) |
| `RABBITMQ_PREFETCH` | `10` | сколько задач воркер держит одновременно |

DSN для Postgres и URL для RabbitMQ собираются в коде из отдельных полей (`config.PostgresConfig.DSN()` и `config.RabbitMQConfig.URL()`). Отдельных переменных `POSTGRES_DSN` и `RABBITMQ_URL` нет — это сделано, чтобы приложение и docker-compose использовали одни и те же значения без дублирования.

## Структура

```
cmd/                — точки входа (server, worker)
internal/
├── app/            — сборка зависимостей
├── broker/         — RabbitMQ (клиент, publisher, consumer)
├── config/         — конфигурация
├── domain/         — сущности и контракты
├── repository/     — PostgreSQL
├── services/       — бизнес-логика
├── storage/        — MinIO
├── transport/      — HTTP
└── worker/         — обработчики фоновых задач
migrations/         — SQL-миграции
web/                — веб-интерфейс
tests/              — E2E-тесты и фикстуры
```

## Команды

```
make help        # список всех команд
make run         # сервер
make worker      # воркер
make test        # тесты
make lint        # golangci-lint
make infra-up    # postgres + minio + rabbitmq
make infra-down  # остановить инфраструктуру
make docker-up   # всё в Docker
make docker-down # остановить Docker
```

## Лицензия

MIT
