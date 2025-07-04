FROM golang:1.24.2-alpine AS builder

ENV CGO_ENABLED=0 GOOS=linux
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -ldflags="-w -s" -o /app/compiler-service ./cmd/main.go

FROM alpine:latest

RUN apk add --no-cache ca-certificates docker-cli

WORKDIR /app

COPY --from=builder /app/compiler-service .

EXPOSE 703

CMD ["./compiler-service"]
