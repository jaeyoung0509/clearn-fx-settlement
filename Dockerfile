FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/api ./cmd/api

FROM alpine:3.20

WORKDIR /app
COPY --from=builder /bin/api /usr/local/bin/api

EXPOSE 8000

ENTRYPOINT ["api"]
