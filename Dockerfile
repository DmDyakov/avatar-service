# Build stage
FROM golang:1.27-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/worker ./cmd/worker

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/

COPY --from=builder /bin/server .
COPY --from=builder /bin/worker .
COPY --from=builder /app/web ./web/
COPY --from=builder /app/migrations ./migrations/

EXPOSE 8080

CMD ["./server"]
