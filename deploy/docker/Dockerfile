# Build stage
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /knowledge-engine ./cmd/server

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /knowledge-engine /app/knowledge-engine
COPY --from=builder /app/migrations /app/migrations

RUN mkdir -p /app/data/docs

EXPOSE 8083

ENTRYPOINT ["/app/knowledge-engine"]
