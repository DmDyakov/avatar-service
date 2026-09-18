.PHONY: help run worker build test test-cover test-race lint fmt tidy generate \
        docker-up docker-down docker-logs docker-ps \
        infra-up infra-down \
        clean

# --- Помощь ---
help: ## Показать список команд
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# --- Запуск ---
run: ## Запустить HTTP-сервер
	go run ./cmd/server

worker: ## Запустить воркер
	go run ./cmd/worker

# --- Сборка ---
build: ## Собрать бинарники server и worker
	go build -o bin/server ./cmd/server
	go build -o bin/worker ./cmd/worker

# --- Тесты ---
test: ## Запустить все тесты
	go test ./... -v

test-cover: ## Тесты с покрытием (HTML-отчёт)
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out

test-race: ## Тесты с race detector
	go test ./... -race

# --- Качество кода ---
lint: ## Запустить golangci-lint
	golangci-lint run

fmt: ## Форматировать код
	go fmt ./...

tidy: ## Привести go.mod в порядок
	go mod tidy

generate: ## Сгенерировать моки (mockgen)
	go generate ./...

# --- Docker ---
docker-up: ## Поднять всё из docker-compose
	docker compose up -d

docker-down: ## Остановить всё
	docker compose down

docker-logs: ## Логи контейнеров (follow)
	docker compose logs -f

docker-ps: ## Статус контейнеров
	docker compose ps

# --- Инфраструктура (только зависимости) ---
infra-up: ## Поднять PostgreSQL + MinIO
	docker compose up -d postgres minio

infra-down: ## Остановить PostgreSQL + MinIO
	docker compose stop postgres minio

# --- Утилиты ---
clean: ## Удалить бинарники и coverage
	rm -rf bin/ coverage.out
