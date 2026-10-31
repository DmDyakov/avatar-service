.PHONY: help run worker build test test-cover test-race lint fmt tidy generate \
        docker-up docker-down docker-logs docker-ps down-v \
        infra-up infra-down \
        clean

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Compose ищет docker-compose.yml и .env в текущей директории (корень)
COMPOSE = docker compose

# --- Помощь ---
help: ## Показать список команд
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# --- Запуск ---
run: ## Запустить HTTP-сервер локально
	go run ./cmd/server

worker: ## Запустить воркер локально
	go run ./cmd/worker

# --- Сборка ---
build: ## Собрать бинарники server и worker
	go build -o bin/server ./cmd/server
	go build -o bin/worker ./cmd/worker

# --- Тесты ---
test: ## Запустить все тесты
	go test ./... -v

test-cover: ## Тесты с покрытием (HTML + итог в терминале)
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1
	go tool cover -html=coverage.out

test-race: ## Тесты с race detector
	go test ./... -race

# --- Качество кода ---
lint: ## Запустить golangci-lint
	@command -v golangci-lint >/dev/null || (echo "golangci-lint not installed" && exit 1)
	golangci-lint run

fmt: ## Форматировать код
	go fmt ./...

tidy: ## Привести go.mod в порядок
	go mod tidy

generate: ## Сгенерировать моки
	go generate ./...

# --- Docker ---
docker-up: ## Поднять всё из compose
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_DATE=$(BUILD_DATE) \
		$(COMPOSE) up -d --build

docker-down: ## Остановить всё (данные сохраняются)
	$(COMPOSE) down

docker-logs: ## Логи контейнеров (follow)
	$(COMPOSE) logs -f

docker-ps: ## Статус контейнеров
	$(COMPOSE) ps

down-v: ## Остановить всё и УДАЛИТЬ данные (volumes)
	@echo "⚠️  Удалит postgres_data, minio_data, rabbitmq_data. Ctrl+C чтобы отменить."
	@sleep 2
	$(COMPOSE) down -v

# --- Инфраструктура (только зависимости) ---
infra-up: ## Поднять PostgreSQL + MinIO + RabbitMQ
	$(COMPOSE) up -d postgres minio rabbitmq

infra-down: ## Остановить инфраструктуру (данные сохраняются)
	$(COMPOSE) stop postgres minio rabbitmq

# --- Утилиты ---
clean: ## Удалить бинарники и coverage
	rm -rf bin/ coverage.out
