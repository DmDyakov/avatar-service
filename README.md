# Avatar Service

Сервис аватарок: принимает картинки, складывает в S3, в фоне режет миниатюры и отдаёт всё это по HTTP. Сделан под GophProfile — чтобы сторонние сервисы могли получать аватар пользователя по email.

## Стек

Go 1.27, PostgreSQL 16 (метаданные), MinIO/S3 (файлы), RabbitMQ (очередь для воркера), chi (роутер), zap (логи), golang-migrate (миграции). Контракты API — OpenAPI 3.0, смотреть через Swagger UI.

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
make worker      # воркер миниатюр
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

Контракты — в [`internal/api/openapi.yaml`](internal/api/openapi.yaml). Удобнее смотреть через Swagger UI:

```
make openapi-ui
# http://localhost:8081
```

Там все эндпоинты с кодами ответов, схемами и примерами. Нереализованные помечены `❌` в названии — сразу видно, что «уже работает», а что «ещё в планах».

## Веб-интерфейс

Две HTML-страницы в `web/static/`: форма загрузки и (в будущем) галерея. Ванильный JS + Tailwind через CDN — без сборки, `node_modules` и Node в CI.

```
http://localhost:8080/web/upload
```

Форма умеет два режима. С включённым JS — `fetch` к `/api/v1/avatars`, превью под формой без перезагрузки. Без JS — нативный submit на `POST /web/upload`, сервер редиректит на галерею (PRG). Второй режим не декоративный: если скрипт не загрузился или его отключили, форма всё равно работает.

Галерея (`/web/gallery/{user_id}`) — в работе, задача AS-204. Пока ссылка из формы ведёт на 404.

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

DSN для Postgres и AMQP URL собираются в коде из отдельных полей (`config.PostgresConfig.DSN()`, `config.RabbitMQConfig.URL()`), а не из готовых строк. Так пароль с `@` или `:` не ломает парсинг URL, и compose с приложением используют одни и те же значения.

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
├── services/       — бизнес-логика server
├── storage/        — MinIO/S3
├── transport/      — HTTP: сервер, хендлеры, middleware
└── worker/         — бизнес-логика worker (thumbnail handler)
migrations/         — SQL-миграции (встроены в бинарник через go:embed)
web/                — веб-интерфейс
tests/              — e2e и фикстуры
```

## Команды

```
make help              # список всех команд с описаниями
make run               # сервер локально
make worker            # воркер локально
make test              # все тесты (testcontainers поднимает PG, MinIO, RabbitMQ)
make test-cover        # покрытие + падение при <50%
make lint              # golangci-lint
make check             # lint + coverage + openapi — то, что гоняем перед PR
make infra-up          # postgres + minio + rabbitmq
make infra-down        # остановить инфраструктуру (данные останутся)
make docker-up         # собрать и поднять всё в Docker
make docker-down       # остановить
make openapi-validate  # проверить спецификацию
make openapi-ui        # Swagger UI на http://localhost:8081
```

## Лицензия

MIT
