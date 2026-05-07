# Currency Rates Service

Go microservice for Bank.lv ECB exchange rates.

## Tech

- Go 1.26
- Chi
- PostgreSQL 18
- pgx
- Bob for sql/db codegen
- oapi-codegen for OpenAPI codegen
- gofrs uuid v7 
- Redis shared HTTP rate limit store
- Task
- Docker Compose

## Run

Install tools:

```bash
go install github.com/go-task/task/v3/cmd/task@latest
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
go install github.com/stephenafamo/bob/gen/bobgen-psql@latest
go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
```

Start infra and app:

```bash
task env-rebuild
```

Fetch selected currencies:

```bash
task fetch
```

Query API:

```bash
curl http://localhost:8888/api/v1/rates
curl http://localhost:8888/api/v1/rates/GBP/history
curl -X POST -H "Content-Type: application/json" -d '{"currencies":["GBP","USD"]}' http://localhost:8888/api/v1/rates/fetch
```

`POST /api/v1/rates/fetch` stores successful currency rates in PostgreSQL. After it returns, `GET /api/v1/rates` and `GET /api/v1/rates/{currency}/history` read those stored rows.

Run tests:

```bash
task test
```

## Commands

```bash
go run ./cmd/rates-service serve
go run ./cmd/rates-service fetch
```

`fetch` starts one goroutine per selected currency. Each goroutine makes its own request to `https://www.bank.lv/vk/ecb_rss.xml`, extracts only its assigned currency, then ingestion writes successful results to PostgreSQL. The HTTP fetch endpoint uses the same ingestion path. Partial success is saved; command exits non-zero when at least one currency failed.

## Configuration

| Env | Default |
| --- | --- |
| `DB_HOST` | `localhost` |
| `DB_PORT` | `5432` |
| `DB_NAME` | `rates-service` |
| `DB_SCHEMA` | `rates` |
| `REDIS_ADDRESS` | `localhost:6379` |
| `BANK_LV_URL` | `https://www.bank.lv/vk/ecb_rss.xml` |
| `SELECTED_CURRENCIES` | `USD,GBP,JPY,CHF,PLN,SEK,NOK,DKK,CAD,AUD` |
| `FETCH_CONCURRENCY` | `4` |
| `RATE_LIMIT_LIMIT` | `120` |
| `RATE_LIMIT_WINDOW_SECONDS` | `60` |

## API

OpenAPI contract lives in `docs/openapi.yaml`.
