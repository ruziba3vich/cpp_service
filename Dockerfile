FROM golang:1.24.2-alpine AS builder

ENV CGO_ENABLED=0 GOOS=linux
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -ldflags="-w -s" -o /app/compiler-service ./main.go

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/compiler-service .

COPY config.yaml ./
EXPOSE 7774

CMD ["./compiler-service"]
