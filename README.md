# akshay2172/prisma-factsheet

Prisma Factsheet is a small Go service that computes portfolio “factsheets” from seeded holdings and market prices. It includes an ETL pipeline that generates:

- `nav_history`
- `exposure_snapshots`
- `performance_metrics`

…and exposes computed results via a JSON API.

## Features

### ETL pipeline
- **Price Fetch** (`price_fetch`): fetches price history (Yahoo) or uses seeded data as a baseline.
- **NAV Calc** (`nav_calc`): computes NAV as **Σ(weight × adj_close)** using the latest available price on/before the target date.
- **Exposure Calc** (`exposure_calc`): aggregates weights by country, sector, and market-cap tier.
- **Performance Calc** (`perf_calc`): computes return and risk metrics from NAV history.

### Automated scheduling
- The API server runs a cron job daily at **01:00 UTC** (after global market close).

### REST API
- Endpoints for holdings/exposures and performance (see API section below).
- Admin ETL trigger endpoint (see API section).

## Technology stack

- **Go** (Golang)
- **Gin** (HTTP router / middleware)
- **PostgreSQL 15+**
- **Redis** (caching)
- **SQL migrations** via `migrations/001_initial_schema.sql`
- **Robfig/Cron** (scheduled ETL)
- **zerolog** (logging)


## Project structure

- `cmd/api/main.go`
  - Starts the HTTP server, cron scheduling, and routes.
- `cmd/etl/main.go`
  - CLI entry point to run individual ETL steps.
- `internal/pipeline/pipeline.go`
  - `PipelineRunner` with ETL steps: price fetch, NAV, exposures, performance.
- `internal/pipeline/yahoo.go`
  - Yahoo price fetcher implementation.
- `internal/services/factsheet.go`
  - Service layer for reading factsheet data.
- `internal/handlers/handlers.go`
  - HTTP handlers / route registrations.
- `internal/db/db.go`
  - Database connection.
- `migrations/001_initial_schema.sql`
  - Schema + seed data (benchmarks, products, securities, holdings, price history).

## Prerequisites

Ensure you have the following installed:

- Go 1.23+
- Docker + Docker Compose
- Git

## Environment configuration

This project uses environment variables for database/service configuration. A template is provided in `.env.example`.

```bash
cp .env.example .env
```

Key variables:

| Variable | Description | Default (local) |
|---|---|---|
| `DB_HOST` | PostgreSQL host address | `postgres` (Docker) |
| `DB_PORT` | PostgreSQL port | `5432` |
| `DB_USER` | DB username | `postgres` |
| `DB_PASSWORD` | DB password | `postgres` |
| `DB_NAME` | Database name | `prisma_factsheet` |
| `REDIS_ADDR` | Redis connection string | `redis:6379` |
| `PORT` | API server port | `8080` |

## Docker Compose quick start

The `docker-compose.yml` file provisions:

- **PostgreSQL** (`prisma_pg`) — runs SQL in `./migrations` on first startup
- **Redis** (`prisma_redis`)
- **API** (`prisma_api`)

Start the infrastructure:

```bash
docker compose up -d
```

### Service health / initialization

- Postgres uses `pg_isready` health checks.
- Redis uses `redis-cli ping` health checks.
- The API starts only after both dependencies are healthy.


## Getting started

### 0) Clone the repository

```bash
git clone https://github.com/akshay2172/prisma-factsheet.git
cd prisma-factsheet
```

### 1) Start services


```bash
docker compose up -d
```

This starts:
- PostgreSQL (with automatic SQL execution from `./migrations` on first boot)
- Redis
- API server

### 2) (Optional) Run ETL manually

If you just want to populate computed tables immediately, run:

```bash
docker compose --profile etl run etl --job=all --date=2024-01-15
```

### 3) Access the API

Open the API base URL in your browser (for example):

- `http://localhost:8080`

Then use the endpoints below. Example curl:

```bash
curl http://localhost:8080/api/v1/products/global-growth-prisma/performance
```

## Usage


### ETL CLI

Run ETL steps from the CLI:

```bash
go run ./cmd/etl --job all --date 2024-01-15
```

Supported jobs:

- `price_fetch`
- `nav_calc`
- `exposure_calc`
- `perf_calc`
- `all`

Examples:

```bash
go run ./cmd/etl --job price_fetch --date 2024-01-15

go run ./cmd/etl --job nav_calc --date 2024-01-15

go run ./cmd/etl --job exposure_calc --date 2024-01-15

go run ./cmd/etl --job perf_calc --date 2024-01-15
```

### API server

Start the API:

```bash
go run ./cmd/api
```

The server listens on:
- `PORT` env var if set, otherwise `8080`

## API endpoints

Route definitions are in `internal/handlers/handlers.go`. Common endpoints in this project:

- **Holdings**
  - `GET /api/v1/products/global-growth-prisma/holdings`

- **Exposures**
  - `GET /api/v1/products/global-growth-prisma/exposures`

- **Performance**
  - `GET /api/v1/products/global-growth-prisma/performance`
  
  - **factsheet**
  - `GET /api/v1/products/global-growth-prisma/factsheet`

- **Admin ETL trigger**
  - `POST /api/v1/admin/etl/run` with body like:

```json
{
  "pipeline": "all"
}
```

## Seed data for ETL correctness

`migrations/001_initial_schema.sql` includes:

- A `holdings` snapshot for product `global-growth-prisma` as of **2024-01-15**
- Synthetic `price_history` rows for the seeded securities from **2023-12-01 to 2024-01-15**

This ensures ETL steps can compute NAV/exposures/performance without requiring external price data.

## Development guidelines

- Run ETL after applying migrations:
  - `go run ./cmd/etl --job all --date 2024-01-15`
- Keep ETL step order consistent:
  1. `price_fetch` (or seeded prices)
  2. `nav_calc`
  3. `exposure_calc`
  4. `perf_calc`
- Prefer deterministic/synthetic seeds for local dev so the factsheet pages are populated immediately.

## License

MIT (or replace with your preferred license).
