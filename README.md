# NPDMS Backend

Backend services for the **National Police Data Management System** — REST API, database schema and migrations, ML services, and the infrastructure that runs them.

The web frontend lives in a separate repository ([Tatu1984/npdms](https://github.com/Tatu1984/npdms), `ui/web`) and talks to this API over HTTP.

---

## Layout

| Path | What it is |
|------|-----------|
| `services/api` | Go 1.22 / Gin REST API — the main service |
| `services/db/init` | Base PostgreSQL schema + seed data |
| `services/api/migrations` | Incremental SQL migrations (`000006`–`000025`) |
| `services/ml` | Python (FastAPI) ML services: FIR classifier, semantic search, OCR, crime prediction, transcription, video analysis |
| `scripts` | Demo data seeding |
| `infrastructure` | Kubernetes manifests, production compose file |
| `monitoring` | Prometheus config + Grafana dashboards |
| `deploy/vault` | Vault policies and bootstrap |
| `docker-compose.yml` | Full local stack (Postgres, Redis, MinIO, Vault, Redpanda, Neo4j, OpenSearch, ML services, monitoring) |

---

## Running locally

### Option A — natively (fastest)

Requires Go 1.22+, PostgreSQL, and Redis.

```bash
# 1. Database
createdb npdms
psql -d npdms -f services/db/init/001_schema.sql
for f in services/api/migrations/*.up.sql; do psql -d npdms -f "$f"; done

# 2. Redis
redis-server --daemonize yes

# 3. API
cd services/api
cp .env.example .env        # then edit DATABASE_URL and JWT_SECRET
go build -o api . && ./api  # must run from services/api — it reads .env from the cwd
```

The API listens on `http://localhost:8080`.

### Option B — Docker

```bash
cp services/api/.env.example .env
./start.sh dev          # infrastructure only
./start.sh ml           # + ML services
./start.sh monitoring   # + Prometheus and Grafana
./start.sh full         # everything
```

---

## Health and auth

```bash
curl http://localhost:8080/health   # {"status":"healthy",...}
curl http://localhost:8080/ready    # reports database + redis status

curl -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"Demo@123"}'
```

The base schema seeds demo users (`admin`, `constable`, `hc`, `asi`, `si`, `inspector`, `sho`, `dsp`). Their stored hash is a placeholder — set a real one before first login:

```bash
# generate with bcrypt, then:
psql -d npdms -c "UPDATE users SET password_hash='<bcrypt-hash>' WHERE username='admin'"
```

All routes below `/api/v1` except `/auth/*` and `/public/*` require `Authorization: Bearer <accessToken>`.

---

## API surface

`/api/v1` — auth, firs, cases, evidence, warrants, bail, forensics, personnel, vehicles, court (hearings/orders), alerts, lookout, cyber-crime, traffic challans, citizen portal, reports, analytics, sync, biometrics, AI review, uploads, and the national/state/district hierarchy.

Run the API and read the `[GIN-debug]` route table at startup for the full list.

---

## Configuration

See `services/api/.env.example` for every variable, and `ENV_VARIABLES.md` for details. Required: `DATABASE_URL`, `JWT_SECRET`, `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`. The API starts without MinIO reachable — file uploads fail gracefully until it is available.

`MAX_CONCURRENT_SESSIONS` (default `3`) caps simultaneous sessions per user; raise it for local development.

---

## Known state

- `internal/services/sync_service.go` is excluded from the build (`//go:build ignore`). It targets a `SyncRepository` API that was never implemented and nothing references it. Federated sync needs that repository layer written before the file can come back.
- `testutil/fixtures.go` does not compile — its struct literals are out of date with `internal/models`, so `go test ./...` fails to build. The API binary itself is unaffected.
- `services/db/init/001_schema.sql` predates several migrations, so some `CREATE TABLE IF NOT EXISTS` migrations are skipped against a fresh database and leave tables missing columns the code selects. The schema and the migration chain should be reconciled into one source of truth.

## Deployment

See `DEPLOYMENT.md`. `services/api` carries `fly.toml` and `railway.toml`; `infrastructure/kubernetes` holds K8s manifests.
