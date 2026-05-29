# ── Build stage ───────────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/api  ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/etl  ./cmd/etl

# ── API runtime ───────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12 AS api
COPY --from=builder /bin/api /api
EXPOSE 8080
ENTRYPOINT ["/api"]

# ── ETL runtime ───────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12 AS etl
COPY --from=builder /bin/etl /etl
ENTRYPOINT ["/etl"]