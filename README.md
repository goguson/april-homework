# Currency Rates Service

Go microservice for Bank.lv ECB exchange rates.

## API / UI

OpenAPI contract lives in `docs/openapi.yaml` and URL: http://localhost:8888/api/docs/

## What else could/would be done

- I find it hard to balance between providing good enough solution for such tasks (recruitment homework) and not spending too much time/effort, as each of us focus on different things. I wanted to avoid any unecessary over-engineering like making sure db is as fast as possible, making sure I choose the most suitable DB engine for given write/read patterns and simillar.
- As a base I used my service template, so there are kinda not needed things like rate-limiting with state backed in redis (for rate limmiting across multiple isntances) or some health endpoints. 

### Database
- even with scale for such usecase postgres would be just fine. We could make use of partitioning or go all in with plugins like TimescaleDB
- I used some niche lib called Bob, I find it sometimes usefull, as it generates orm-like tooling without a real cost. It can bu used next to PGX/sqlc just fine, does not vendor-lock-in our selfes no matter how much it is used. We can always fall back to just pgx, yet we gain a lot of cool generated funcs/tooling for fast iterations and quick queries on demand, out of generated code.
  
### Tests
- I would not say there are really meaningful tests, but as it is just a homework I did not invest much time into more proper tests.
- I guess I would introduce some component/integration tests as well as make use of Testcontainers.

### API
- it is not the most beautiful implementation code-wise, but it does the job for now. 

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



