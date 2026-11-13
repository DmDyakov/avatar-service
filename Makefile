.PHONY: help run worker build test test-cover test-cover-full test-race lint check fmt tidy generate \
        docker-up docker-down docker-logs docker-ps down-v \
        infra-up infra-down \
        openapi-validate openapi-ui \
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
test: ## Запустить все тесты (с testcontainers, нужен Docker)
	go test ./... -v

# Пакеты с бизнес-логикой: их покрытие считаем в -short-прогоне.
# Исключены: cmd/* (main), internal/app (DI), internal/logger (обёртка),
# pkg/buildinfo (ldflags), migrations (go:embed), mocks (generated),
# repository/worker/storage/broker (интеграционные — test-cover-full).
COVERAGE_PKGS ?= ./internal/config/... \
                 ./internal/services/... \
                 ./internal/transport/http/handlers/... \
                 ./internal/transport/http/middleware/... \
                 ./pkg/lifecycle/...

# Дополнительные пакеты для полного прогона с testcontainers.
COVERAGE_PKGS_FULL ?= ./internal/repository/postgres/avatar/... \
                      ./internal/worker/...

# Минимальный порог покрытия (AS-403.2, ТЗ п. 3.1).
COVERAGE_MIN ?= 50

test-cover: ## Тесты с покрытием (-short) + гейт COVERAGE_MIN
	@go test -short $(COVERAGE_PKGS) -coverprofile=coverage.out -covermode=atomic
	@grep -vE "/mocks/" coverage.out > coverage.filtered && mv coverage.filtered coverage.out
	@echo "--- coverage by package ---"
	@go tool cover -func=coverage.out | grep -vE "^total:" | awk '{print $$1}' | \
	  sed 's|/avatar-service/||' | sed 's|:[0-9]*:$$||' | sort -u | head -20 || true
	@echo "--- total ---"
	@go tool cover -func=coverage.out | tail -1
	@PCT=$$(go tool cover -func=coverage.out | tail -1 | awk '{print $$3}' | tr -d '%'); \
	  echo "coverage: $$PCT% (min: $(COVERAGE_MIN)%)"; \
	  if [ $$(echo "$$PCT < $(COVERAGE_MIN)" | bc -l) -eq 1 ]; then \
	    echo "FAIL: coverage $$PCT% < $(COVERAGE_MIN)%"; \
	    exit 1; \
	  fi

test-cover-full: ## Покрытие с testcontainers (медленно, нужен Docker)
	@go test $(COVERAGE_PKGS) $(COVERAGE_PKGS_FULL) -coverprofile=coverage.out -covermode=atomic
	@grep -vE "/mocks/" coverage.out > coverage.filtered && mv coverage.filtered coverage.out
	@go tool cover -func=coverage.out | tail -1
	@go tool cover -html=coverage.out -o coverage.html
	@echo "HTML: coverage.html"

test-race: ## Тесты с race detector
	go test ./... -race

# --- Качество кода ---
lint: ## Запустить golangci-lint
	@command -v golangci-lint >/dev/null || (echo "golangci-lint not installed" && exit 1)
	golangci-lint run

check: lint test-cover openapi-validate ## Полный гейт перед коммитом / PR
	@echo "✓ check passed"

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
clean: ## Удалить бинарники, coverage.out, coverage.html
	rm -rf bin/ coverage.out coverage.filtered coverage.html

# --- OpenAPI ---
openapi-validate: ## Проверить OpenAPI-спеку (docker, python)
	@docker run --rm -v "$(PWD):/spec" python:3-alpine sh -c \
		"pip install -q openapi-spec-validator && openapi-spec-validator /spec/internal/api/openapi.yaml"

openapi-ui: ## Открыть Swagger UI на http://localhost:8081
	@echo "Swagger UI: http://localhost:8081"
	@docker run --rm -p 8081:8080 \
		-e SWAGGER_JSON=/spec/openapi.yaml \
		-v "$(PWD)/internal/api:/spec" \
		swaggerapi/swagger-ui
