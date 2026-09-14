# Oracle Cloud Always Free

A demonstration and staging deployment on Oracle's free `VM.Standard.E2.1.Micro`
shape: **1/8 OCPU and 1 GB RAM**.

This is for showing the platform and rehearsing the deployment. Live case data
belongs on the MDC edge server — see `deploy/edge/`.

## What differs from the edge profile

| | Edge (`deploy/edge/`) | Oracle free (here) |
|---|---|---|
| Object storage | MinIO | Local disk via the filesystem backend |
| Postgres | PostGIS image, default tuning | Plain Postgres, tuned for 1 GB |
| Redis | 512 MB, persistent | 64 MB, no persistence |
| Memory limits | None | Declared per service |
| Swap | Machine-dependent | 2 GB swapfile, created by `setup.sh` |

Phase 02 chain of custody behaves identically — the filesystem and MinIO
backends satisfy the same interface, and the digest is computed the same way.

## Install

On the instance:

```bash
git clone git@github.com:Tatu1984/npdms-backend.git
cd npdms-backend
bash deploy/oracle-free/setup.sh     # swap, Docker, instance firewall
# log out and back in so docker group membership applies
cd deploy/oracle-free
cp .env.example .env                 # fill in every CHANGE_ME
docker compose up -d --build
```

Then create the schema:

```bash
DATABASE_URL='postgresql://npdms:<password>@localhost:5432/npdms?sslmode=disable' \
  ../../scripts/bootstrap-db.sh --with-demo-data
```

Verify:

```bash
curl -s localhost:8080/health
curl -s localhost:8080/ready
```

## The two firewalls

Oracle blocks ports in **two** places and both must be opened:

1. **VCN Security List / NSG** — in the OCI console. Not scriptable from the
   instance.
2. **The instance's own firewall** — `firewalld` on Oracle Linux, `iptables` on
   their Ubuntu images. `setup.sh` handles this one.

A port open in only one of them fails silently, which is the most common reason
a working service appears unreachable.

## Living within 1 GB

- **`docker compose build` is the heaviest thing that will run here.** The Go
  build can exhaust memory even with swap. If it fails, build the image
  elsewhere and `docker save` / `docker load` it across, or build with
  `docker compose build --memory 512m`.
- Check headroom with `free -h` and `docker stats --no-stream`.
- If Postgres is killed by the OOM killer, the swapfile is missing or
  `shared_buffers` has been raised. Check `dmesg | grep -i oom`.

## What this box will not do

The ML services (Phase 03 video, OCR, transcription) need several GB and, for
video, a GPU. They do not belong here. Neither does anything with a real chain
of custody attached to it.

## Idle reclamation

Oracle reclaims Always Free compute that looks idle. A box serving occasional
demo traffic can be flagged. If the instance disappears, that is the likely
reason — the data is on a block volume and survives, but plan for it.
