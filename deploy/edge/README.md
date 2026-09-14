# Edge deployment

Single-node deployment for an MDC / AI Edge server. One box runs the API,
Postgres, Redis and object storage. There is no cloud dependency and no second
node — the database on this server is the system of record.

## Install

```bash
cp .env.example .env          # fill in every CHANGE_ME
docker compose up -d
```

Then create the schema. The migration chain has undeclared order dependencies,
so it is applied by the bootstrap script rather than by the Postgres image's
entrypoint:

```bash
docker compose exec -T postgres bash -c '
  DATABASE_URL="postgresql://$POSTGRES_USER:$POSTGRES_PASSWORD@localhost:5432/$POSTGRES_DB?sslmode=disable" \
  /opt/npdms/bootstrap-db.sh'
```

or, running it from the repository against the container's exposed port:

```bash
DATABASE_URL='postgresql://npdms:<password>@localhost:5432/npdms?sslmode=disable' \
  ../../scripts/bootstrap-db.sh --with-demo-data
```

Verify:

```bash
curl -s localhost:8080/health   # {"status":"healthy",...}
curl -s localhost:8080/ready    # reports database and redis
```

## Offline installation

The box does not need internet access at runtime. To provision one that has
none:

1. On a connected machine, pull and export the images:
   ```bash
   docker compose pull
   docker save postgis/postgis:16-3.4 redis:7-alpine \
     minio/minio:RELEASE.2024-06-13T22-53-53Z -o npdms-images.tar
   ```
2. Build the API image and export it the same way.
3. Copy `npdms-images.tar` and this repository to the edge server.
4. `docker load -i npdms-images.tar`, then `docker compose up -d`.

Set `IP_INTEL_ENABLED=false` on an air-gapped box.

## Backup

`scripts/backup.sh` writes a consistent dump of the database and the object
store. Schedule it; an edge server has no replica to fall back on.

```bash
../../scripts/backup.sh /var/backups/npdms
```

Restore is documented at the top of that script.

## What this deliberately does not run

The repository root `docker-compose.yml` also defines Vault, Redpanda, Neo4j,
OpenSearch, Prometheus, Grafana and the Python ML services. That is the full
research stack. An edge server does not need a message bus or a second graph
database, and the ML services are a later phase. Add them only when a specific
module requires one.

## Exposure

The API binds to `API_BIND` (default `0.0.0.0:8080`) so the station LAN can
reach it. Postgres, Redis and MinIO bind to `127.0.0.1` only and must stay that
way. Put a reverse proxy with TLS in front of the API before any traffic
crosses a network you do not control.
