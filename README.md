# Avatar Service

Микросервис для управления аватарками пользователей.

## Стек

- Go 1.27
- PostgreSQL
- chi (HTTP router)
- zap (logger)
- golang-migrate (миграции)

## Быстрый старт

### 1. Поднять PostgreSQL

```bash
make docker-up
```

### 2. Настроить окружение

```bash
cp .env.example .env
# отредактировать .env под себя
```

### 3. Запустить сервер

```bash
make run
```

Сервер будет доступен на `http://localhost:8080`.

### 4. Проверить health

```bash
curl http://localhost:8080/ping
```

## Команды

| Команда | Описание |
|---------|----------|
| `make run` | Запустить сервер |
| `make worker` | Запустить воркер |
| `make build` | Собрать бинарники |
| `make test` | Запустить тесты |
| `make lint` | Запустить линтер |
| `make docker-up` | Поднять PostgreSQL |
| `make docker-down` | Остановить контейнеры |
