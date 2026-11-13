# Avatar Service

Сервис аватарок: принимает картинки, складывает в S3, в фоне режет миниатюры и отдаёт всё это по HTTP. Сделан под GophProfile — чтобы сторонние сервисы могли получать аватар пользователя по email.

## Стек

Go 1.27, PostgreSQL 16 (метаданные), MinIO/S3 (файлы), RabbitMQ (очередь для воркера), chi (роутер), zap (логи), golang-migrate (миграции). Rate limiting — `golang.org/x/time/rate` (token bucket per-IP). Интеграционные тесты — `testcontainers-go`. Контракты API — OpenAPI 3.0, смотреть через Swagger UI.

## Запуск

Перед первым стартом — скопировать шаблон конфига:

```
cp .env.example .env
```

Дальше два пути — локально или всё в Docker.

### Локально

Инфраструктуру (БД, хранилище, брокер) держим в Docker, а приложение запускаем на хосте. Так итерировать быстрее: `go run` не требует пересборки образа.

Три терминала:

```
make infra-up    # postgres, minio, rabbitmq
make run         # сервер на :8080
make worker      # воркер миниатюр и очистки S3
```

Сервер отвечает на `/live` и `/ready`, форма загрузки — по адресу `http://localhost:8080/web/upload`. Когда надоест — `make infra-down`, данные в volumes останутся.

### Всё в Docker

Один заход, всё сразу:

```
docker compose up -d --build
docker compose logs -f server worker
```

`down` останавливает контейнеры, `down -v` — плюс сносит данные (Postgres, MinIO, RabbitMQ). `-v` необратимый, если что.

## API

Все эндпоинты из ТЗ реализованы. Контракты — в [`internal/api/openapi.yaml`](internal/api/openapi.yaml). Удобнее смотреть через Swagger UI:

```
make openapi-ui
# http://localhost:8081
```

Там все эндпоинты с кодами ответов, схемами и примерами. Если что-то в планах — помечено `❌` в названии (актуальный статус всегда в Swagger, не в README).

## Веб-интерфейс

Две HTML-страницы в `web/static/`: форма загрузки и галерея. Ванильный JS + Tailwind через CDN — без сборки, `node_modules` и Node в CI.

```
http://localhost:8080/web/upload    # форма
http://localhost:8080/web/gallery/{user_id}  # галерея
```

**Форма** умеет два режима. С включённым JS — `fetch` к `/api/v1/avatars`, превью под формой без перезагрузки. Без JS — нативный submit на `POST /web/upload`, сервер редиректит на галерею (PRG). Второй режим не декоративный: если скрипт не загрузился или его отключили, форма всё равно работает. После успешной загрузки под превью появляется ссылка «Посмотреть все» на галерею этого пользователя.

**Галерея** — сетка карточек (CSS grid, адаптивная): картинка, имя файла, дата, формат. На каждой карточке — кнопка удаления (красная корзина, `confirm` перед `DELETE`). Пустой список — сообщение с кнопкой «Upload avatar». Пока показывает **оригиналы**: параметр `?size=100x100` появится в Phase 2 (AS-602), тогда переключим `src` на миниатюры.

## OpenAPI и Swagger UI

Спека лежит в репозитории как документ — не раздаётся сервером (это задача Phase 2).

```
make openapi-validate   # проверить, что YAML валиден
make openapi-ui         # поднять Swagger UI на :8081
```

При изменении API правьте `openapi.yaml` в том же коммите, что и код. `make check` включает `openapi-validate` — сломанную спеку не пропустит.

## Конфигурация

Все переменные, их дефолты и пометки «только для dev» / «для прода» — в [`.env.example`](.env.example). Копируется в `.env` перед запуском.

Секреты (`POSTGRES_PASSWORD`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `RABBITMQ_PASSWORD`) — `required`, без дефолтов. Не задали — приложение падает на старте с внятной ошибкой. Это лучше, чем тихо подняться с паролем `postgres` и потом гадать, почему в проде дыра.

DSN для Postgres и AMQP URL собираются в коде из отдельных полей (`config.PostgresConfig.DSN` и `config.RabbitMQConfig.URL`, заполняются в `Load()`), а не из готовых строк. Так пароль с `@` или `:` не ломает парсинг URL, и compose с приложением используют одни и те же значения.

Лимиты rate limit (`RATE_LIMIT_RPS`, `RATE_LIMIT_UPLOAD_RPS`, …) и CORS (`CORS_ALLOWED_ORIGINS`) — тоже через env. Меняются без пересборки образа: правишь `.env`, `docker compose up -d server`.

## Защита

**Rate limiting** — per-IP, token bucket. Общий лимит на `/api/v1/*` (10 RPS / burst 20), отдельный, более строгий — на `POST /api/v1/avatars` (2 RPS / burst 5). Превышение — `429` с `Retry-After: 1`. `/live`, `/ready`, `/web/*` не ограничены (K8s probes, статика).

**CORS** — только для `/api/v1`. Разрешённые origins из `CORS_ALLOWED_ORIGINS`. Preflight (`OPTIONS`) проходит до rate limit (иначе браузер не даст отправить POST). `*` не используется: API требует `X-User-ID`, а с wildcard браузер не отправит custom header.

**Валидация** — magic bytes (JPEG/PNG/WebP, не по `Content-Type`), лимит 10 MB (`MaxBytesReader` до `ParseMultipartForm`), `X-User-ID` по regex `^[a-zA-Z0-9_\-]+$`, ≤255 символов.

## Структура

```
cmd/                — main для server и worker
internal/
├── api/            — OpenAPI-спецификация
├── app/            — сборка зависимостей: App (server), WorkerApp
├── broker/         — RabbitMQ: client, publisher, consumer, topology
├── config/         — конфигурация из env
├── domain/         — сущности и контракты
├── logger/         — zap с dev/prod форматом
├── repository/     — PostgreSQL: пул, миграции, репозитории
├── services/       — бизнес-логика server (avatar, health)
├── storage/        — MinIO/S3
├── transport/      — HTTP: сервер, хендлеры, middleware (rate limit, CORS)
└── worker/         — бизнес-логика worker: thumbnail handler, delete handler
migrations/         — SQL-миграции (встроены в бинарник через go:embed)
web/                — веб-интерфейс (форма, галерея)
tests/              — e2e и фикстуры
```

## Команды

```
make help              # список всех команд с описаниями
make run               # сервер локально
make worker            # воркер локально
make test              # все тесты (testcontainers: PG, MinIO, RabbitMQ)
make test-cover        # покрытие по бизнес-пакетам + падение при <50%
make test-cover-full   # покрытие с testcontainers (медленно, нужен Docker)
make lint              # golangci-lint
make check             # lint + test-cover + openapi-validate (перед PR)
make infra-up          # postgres + minio + rabbitmq
make infra-down        # остановить инфраструктуру (данные останутся)
make docker-up         # собрать и поднять всё в Docker
make docker-down       # остановить
make down-v            # остановить и снести данные (volumes)
make openapi-validate  # проверить спецификацию
make openapi-ui        # Swagger UI на http://localhost:8081
```

## Лицензия

MIT
