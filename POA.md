# Plan of Action

**Kolkata Police Digital Intelligence Platform**
One platform, fourteen phased modules, for Kolkata Police / West Bengal Police / CID / Traffic Police and the West Bengal courts.

| | |
|---|---|
| Canonical location | `npdms-backend/POA.md` — the frontend repo points here |
| Last updated | 2026-09-16 |
| Current focus | All fourteen phases functional without AI or blockchain, merged to main and deployed (Vercel + Neon), with a fictional Kolkata demo dataset. Next: retire the remaining legacy mock screens, extend the demo data to Phases 11–14, then the open decisions below before the AI and anchoring layers |

---

## How this document is used

This is the live plan. **It is updated every time a task completes** — status changed, work moved between sections, and a line added to the changelog at the bottom. If the plan and the code disagree, the code is right and this file is stale; say so and it gets fixed.

Status values:

| | Meaning |
|---|---|
| `DONE` | Built, verified, and in the repository |
| `IN PROGRESS` | Actively being built |
| `NEXT` | Agreed as the next piece of work |
| `PLANNED` | Scoped, not started |
| `BLOCKED` | Waiting on a decision or an external dependency |

---

## Architecture, decided

These are settled. Work that contradicts them is wrong, not a variation.

**Strictly backend-API oriented.** The Go API is the system of record. Every client — the web application, any mobile client, CCTNS/ICJS, any MDC system — reaches the platform only through the API. No business logic, derived figure or authorisation decision lives outside it. A number the UI needs is a number the API returns.

**Single central edge server.** One box at MDC / AI Edge serves all stations over the network. No federation, no vector clocks, no multi-node conflict resolution — that machinery has been removed rather than maintained.

**Local Postgres on the box.** The database sits beside the API. Works air-gapped, keeps data in jurisdiction, and measured ~4.4 ms against ~280 ms for the cloud database it replaced. Neon is a scratch environment only.

**Every phase works before AI or blockchain.** Each module is made functional on real data with real workflows first. AI and blockchain are layers added afterwards, over modules that already work without them.

**Bilingual throughout.** English and বাংলা are both first-class, wired into the shell from the start rather than retrofitted.

**No dead ends in the UI.** Every button, kebab menu and follow-up action leads to a dialog, a sheet or a route. Enforced by the type system: an action carries either an `href` or an `onSelect`, so a dead control fails to compile.

---

## Foundation

Work that is not a phase but that every phase depends on.

| Item | Status | Notes |
|---|---|---|
| Repo split — backend and frontend | `DONE` | `Tatu1984/npdms-backend` (Go API, schema, migrations, infra) and `Tatu1984/npdms` (`ui/web`). Backend files were not removed from the frontend repo. |
| UI/UX reconstruction | `DONE` | Design tokens (light govt-standard default, dark for ops), shadcn primitives on Radix, reactbits motion, bilingual shell, module registry, WB/Kolkata reference data, BNS/BNSS/BSA replacing IPC. All 14 module screens navigable. |
| Local Postgres + reproducible bootstrap | `DONE` | `scripts/bootstrap-db.sh` — idempotent, offline, two-pass migration apply. 101 tables clean from scratch. |
| Kolkata demo dataset, no records in migrations | `DONE` | Migrations and the base schema now create schema and reference data only; the Koramangala station, `@karpolice.gov.in` accounts, KOR/2024 FIRs and Karnataka/Maharashtra hierarchy seeds are gone, and migration `000064` converts databases built before the change while keeping the demo logins. `services/db/init/002_demo_kolkata.sql` loads 17 real Kolkata Police stations and 23 FICTIONAL demo officers (password `Demo@123`). `scripts/seed-kolkata-demo.py` loads the demo records through the API — see "Demo dataset" below. `scripts/cleanup-test-data.sql` removes probe-tagged rows for review before use. |
| Migration chain repaired | `DONE` | Five real bugs fixed (partitioned PK, non-immutable index predicates, six wrong column names, undeclared ordering). Drift captured in migration `000027`. |
| Edge deployment | `DONE` | `deploy/edge/` — compose with only Postgres, Redis, MinIO, API. Datastores bind to loopback. Offline install path. `IP_INTEL_ENABLED=false` for air-gapped boxes. |
| Free demo/staging deployment — Oracle | `ABANDONED` | `deploy/oracle-free/` remains in the repo and is still correct, but the Oracle Always Free shape was dropped. See the note below. |
| Staging deployment — Vercel | `DONE` | Go API live at `https://api-black-pi.vercel.app`, Vercel project `api`, region `bom1`, database Neon. Demo and integration only; **not** the system of record. See "Deployed environments". |
| Backup and restore | `DONE` | `scripts/backup.sh` — verifies the dump before pruning, states plainly when evidence files are not included. |
| API contract and versioning | `DONE` | `docs/api/CONTRACT.md`, `docs/api/openapi.yaml`, `docs/api/routes.txt` (all 292 routes). Served live at `GET /openapi.yaml`. |
| Audit trail | `DONE` | Hash-chained append-only `audit_logs`. Every state change appended; failures logged, never discarded. |
| Federation code removed | `DONE` | 683 lines of browser-side vector-clock sync deleted, plus its dead backend twin. Not needed on a single server. |
| IP/OSINT lookup moved server-side | `DONE` | `GET /intel/ip/{ip}` — controlled egress, audited with stated purpose, private addresses classified locally. |
| **Core records onto the API** | `DONE` | All thirteen stores retired. FIR, cases, evidence, warrants, bail, court, forensics, personnel, vehicles, alerts, armoury, lookouts and the access log run on the API, each verified in a browser with writes confirmed in Postgres. See below. |
| RBAC and permissions model | `PLANNED` | Role checks exist per-route; needs a coherent model documented and enforced centrally. |
| Offline / sync layer | `PLANNED` | Deferred by decision. The previous IndexedDB layer was removed: it masked failures with demo data, returned locally queued writes as successes, and the service worker cached every API response for 24 hours. When this is built, a queued write must be visibly pending, never shown as saved. |
| CCTNS / ICJS integration | `PLANNED` | The platform consumes authorised data from systems Kolkata Police already operates. Needs their interface specifications. |
| TLS / reverse proxy | `PLANNED` | Still required for the MDC box. The Vercel staging tier terminates TLS itself, so this is outstanding only for the edge deployment. |

### Demo dataset

Fictional people, real places. Loaded through the API as the officer who would record each item, so numbering, workflow rules, custody signatures and the audit trail are genuine. Idempotent through a `demo_seed_ledger` table in the target database.

| Module | Records |
|---|---|
| Stations, officers | 17 Kolkata Police stations across 8 divisions; 23 demo accounts; 20 personnel records; 8 fleet vehicles |
| FIRs and cases | 36 FIRs over six months at 8 stations (BNS / IT Act sections), 16 cases, 13 accused, 10 witnesses |
| Evidence | 16 register items with attached files, verified digests and 28 signed custody legs; 6 forensic requests to CFSL Kolkata, FSL West Bengal and the State Finger Print Bureau |
| Court | 6 warrants, 5 bail applications, 10 hearings and 5 orders at City Sessions, Bankshall, Alipore, Special POCSO, Special Court (Cyber) and Calcutta High Court |
| Phase modules | 4 investigation workspaces, 5 alerts, 4 lookouts with sightings, 10 weapons with 5 issuances, 3 missing-person reports (one child), 4 cyber fraud complaints with a money trail, freeze and recovery, 2 traffic incidents with an approved report, 4 dispatch incidents, 4 citizen complaints (English and Bengali), 8 CCTV cameras with 2 events, 4 risk beats |

Loading it:

```
# empty database: schema, reference data, stations and demo accounts
DATABASE_URL=… ./scripts/bootstrap-db.sh --with-demo-data
# an API running against that database, then
DEMO_SEED_CONFIRM=yes DATABASE_URL=… API_URL=https://…/api/v1 python3 scripts/seed-kolkata-demo.py
```

The seed needs `psql` and Python 3 only. It backs off on rate limiting (a fresh run against a rate-limited API takes about five minutes) and a second run creates nothing. Not included yet: knowledge documents (Phase 11), case files (Phase 12) and body-worn camera records (Phase 13).

### Deployed environments

Two tiers, deliberately different. The architecture decisions above are unchanged: the MDC box with local Postgres is the system of record, and **Neon is a scratch environment only**. The Vercel tier exists so the frontend has a real API to build against, not to hold case data.

| | Staging (Vercel) | Edge (MDC box) |
|---|---|---|
| API | `https://api-black-pi.vercel.app` | `deploy/edge/`, not yet provisioned |
| Vercel project | `api` (Go runtime, region `bom1`) | — |
| Database | Neon `ep-fancy-rain-admoirx7-pooler`, db `neondb` | Local Postgres beside the API |
| Frontend | `https://npdms.infinititechpartners.com` (project `npdms`) | — |
| Evidence files | **Do not persist** — see below | MinIO or local disk |

Verified on the staging tier: `GET /health` returns `200 healthy`, `GET /ready` reports `database: healthy`, and the CORS preflight for `POST /api/v1/auth/login` returns `204` advertising `X-CSRF-Token`. **A real login has not been exercised** — no seeded credentials to hand. The database holds 99 tables, 9 users and 0 cases.

Environment variables, staging API (Vercel project `api`):

```
DATABASE_URL           Neon pooler URL, sslmode=require
JWT_SECRET             set
CUSTODY_SIGNING_KEY    set, separate from JWT_SECRET by design
CORS_ALLOWED_ORIGINS   https://npdms.infinititechpartners.com
MINIO_ENDPOINT         disabled
MINIO_ACCESS_KEY       disabled
MINIO_SECRET_KEY       disabled
STORAGE_BACKEND        filesystem
STORAGE_PATH           /tmp/evidence
ENV                    production
GIN_MODE               release
MAX_CONCURRENT_SESSIONS 5
IP_INTEL_ENABLED       true
```

Environment variables, frontend (Vercel project `npdms`): `NEXT_PUBLIC_API_URL=https://api-black-pi.vercel.app/api/v1` and `NEXT_PUBLIC_DEBUG=false`. Those are the only two needed — `NODE_ENV` is set by Vercel, and `NEXT_PUBLIC_NLP_SERVICE_URL` is left unset until `services/ml` is deployed. `NEXT_PUBLIC_*` is compiled in at build time, so changing it requires a rebuild, not just a save.

Three things known to be wrong on the staging tier, none blocking:

- **Evidence uploads do not persist.** Vercel's filesystem is read-only apart from `/tmp`, which is wiped between invocations. `STORAGE_PATH` is set so the service boots, not because uploads survive. Real storage needs S3/R2, or the edge box. `STORAGE_BACKEND=database` (branch `feat/mpboard`, migration `000070`) now makes files up to 8 MB persist in Postgres — enough for photographs and document scans — and refuses larger ones (recordings) with a message that object storage is required.
- **`DATABASE_URL` must not carry `channel_binding=require`.** `main.go` opens the database twice — pgx and sqlx via `lib/pq` — and `lib/pq` has no channel-binding support.
- **Redis is absent**, so `/ready` reports `not ready` and every cold start burns a 5-second Redis ping timeout. Sessions and rate-limit counters degrade rather than fail.

Why Oracle Always Free was dropped: the `VM.Standard.E2.1.Micro` shape reports 1 GB but has **498 MB usable**, against a profile budgeted at roughly 550 MB. It wedged twice in one session — once merely installing packages — in a way that leaves the kernel answering TCP on port 22 while sshd never sends a banner, so it looks like a network fault rather than an out-of-memory event. Recovery needs a forced `RESET` from the console, which a graceful reboot will not do. `deploy/oracle-free/` is kept because the profile itself is sound; it needs an Ampere A1 shape (4 OCPU / 24 GB, also Always Free) rather than the micro.

---

### Core records onto the API — `DONE`

The largest outstanding foundation item.

**Correction to the earlier scoping.** It described the endpoint-backed modules as "pure rewiring". Two things made that wrong:

- **The legacy React Query hooks are not API-backed in any meaningful sense.** All fifteen in `ui/web/src/hooks/` (`use-firs`, `use-cases`, `use-warrants` and the rest) fall back to IndexedDB and then to hardcoded Bangalore demo records when a request fails, and a failed write is queued locally and returned as if it succeeded. They use raw `fetch` without the `X-CSRF-Token` header. FIR, cases, evidence and alerts screens use these hooks — so they were never on the API either, despite their stores being unused. Each module needs a typed `lib/api/<module>.ts` mirroring the Go models and thin hooks, in the Phase 01/02 pattern.
- **The backend for these modules did not work.** Found by exercising every create, list, get, update and status action against local Postgres, then reading each back from a row with only its required columns set.

| | Modules | State |
|---|---|---|
| Backend verified (57/58 calls, sparse-row reads) | all eleven: cases, warrants, bail, forensics, personnel, vehicles, court hearings, court orders, fir, evidence, alerts | `DONE` — the one failure is alert acknowledge requiring `acknowledgedBy` in the body rather than taking it from the session |
| Frontend on typed clients, verified in a browser | fir, cases, warrants, bail, court, forensics, personnel, vehicles | `DONE` |
| Frontend on typed clients | evidence, alerts | `DONE` — `/evidence` routes to the Phase 02 custody register rather than duplicating it with unsigned transfers |
| Armoury backend | weapon register, issue/return ledger with rounds accounting | `DONE` — migration `000034`, `/armoury/*`. One open issue per weapon enforced by a unique partial index; a return with fewer rounds records the shortfall; a damaged return moves the weapon to maintenance. |
| Lookout backend | notices, sightings, independent verification, resolution | `DONE` — migration `000034`, `/lookouts/*`. A sighting cannot be verified by its reporter (checked in the service and by a table constraint); resolved notices accept no further sightings. |
| Access-log backend | sign-ins, failures, sign-outs with IP and user agent | `DONE` — `/access-log`, DSP and above. No new table: events are written to and read from the immutable audit trail, so the two cannot disagree. Failed sign-ins are attributed to the targeted account. Suspicious sources are a stated rule — five failures from one address within an hour. Sign-ins were not audited at all before this. |
| Armoury, lookout, access-log screens | onto the new endpoints | `DONE` — verified in a browser with two officers |
| Correctly client-side | auth (already API-backed), toast | None |

Known and not yet fixed:
- **Alert scope is not restricted by rank** — an SHO can issue a NATIONAL alert. Belongs with the RBAC model.
- **Evidence status vocabulary is mixed** — legacy rows are `COLLECTED`, custody registration writes `IN_CUSTODY`.
- **Rate limit will throttle a real station.** The global limiter allows 100 requests a minute per IP and runs before authentication. On the single central server, a station behind NAT shares one IP, and each screen issues about five requests per load. Needs a decision: per-user limits after auth, with per-IP kept only for unauthenticated routes.
- **`GET /firs` ignores its date, officer and station filters**, and FIR update does not persist incident date and time or complainant ID fields.
- **Bail stores only `accused_id`.** The frontend now selects from the case's accused register; the API still accepts and discards a free-text name.
- **Bail search matches only the application number**; bail stats merge approved with released and rejected with cancelled.
- **Forensics has no request number column** (`RequestNumber` is always empty) and search covers only the lab. `GET /evidence` ignores search and case filters.
- **Court orders cannot be updated or deleted**; `pendingOrders` is every order recorded, as orders have no pending state.
- **Personnel:** nothing prevents two records for one user account; `assignedCases` is a stored number nothing maintains; assigning duty leaves leave fields set.
- **The accused list returns `null` for none**, and a created accused returns zero-value timestamps.
- **Counters not yet applied** to cyber crime, citizen complaints, grievances, missing-person reports and FIR copy requests — all still count-based or clock-based.
- **Remaining vehicle, trip, fuel and maintenance logs, sureties, and a beat register** do not exist; the UI no longer pretends they do.
- **`court_hearings` carries duplicate columns** from the base schema — `court_name`/`court`, `hearing_type`/`type`, `documents_required`/`required_documents`. The code uses the second of each. Harmless now, to be consolidated as `000032` did for warrants.
- **Handlers discard the underlying error.** Every defect below surfaced only as a generic 500. Logging the cause server-side would have shown each in seconds.
- **`testutil/fixtures.go` does not compile**, so `go test ./...` and `go vet ./...` fail before running anything.
- **Staging (Neon) needs migrations `000032`, `000033` and `000034`** before warrants can be created and before any record number is issued safely there.

---

## Open decisions

Raised, not yet ruled on. Neither blocks current work.

| | |
|---|---|
| `Tatu1984/npdms-backend` stays **public** — `DECIDED` 2026-09-14 | Deliberate, for now. No credentials are in it: `.env` is ignored and secrets were kept out of `vercel.json`. The schema, auth logic and chain-of-custody implementation are publicly readable, which is accepted. Revisit before live case data exists. |
| Face recognition: hosting and authorisation — `DECIDED` 2026-09-15 | Build now, host later: without `FR_SERVICE_URL` every face recognition screen says "not connected". Demo only for now: a `DEMO` authorisation runs only on synthetic test faces uploaded by an administrator, refuses real report photos and labels everything "Demo — synthetic faces". Real photos need an ORDER with reference, date and issuing authority. See "Face recognition for missing persons" under Later layers. |
| RBAC model, CCTNS/ICJS interfaces, ML hardware sizing | Still open, tracked in Foundation and Later layers above. |

---

## The fourteen phases

Each phase is complete when its workflows run on real data through the API, with an audit trail — **not** when its screens render.

The UI for every phase already exists and is navigable; where a phase is not `DONE`, those screens are driven by fixture data in `ui/web/src/lib/platform/mock.ts` and are a specification of the intended behaviour, not working software.

---

### Phase 01 — AI Investigation Copilot · `DONE` (complete)

> **Verified through the UI, 2026-09-15.** An earlier pass marked this done after API-only checks; the forms could not be filled in a browser (fixed in `79dfa5e`). Every capability below has now been driven through the real screens — 35 browser checks plus a 17-check API probe, each write confirmed in Postgres (`46e71f9` frontend, `9e9f596` backend). Covered: workspace create, search, edit, status and court date, and reassignment with the reason audited; persons added, edited and removed with age, aliases, phones and vehicles; chronology with sources and location; contradictions recorded and reviewed, showing the actual reviewer; tasks created with assignee and due date, moved through every status, and deleted; evidence attached and detached from the register; officer-raised gaps; all rule behaviour — new-workspace gaps, each closing when resolved, the >60-minute interval gap, a dismissed gap staying dismissed, and completing a task on a rule gap re-running the rules instead of force-closing it; counts, relationship graph, brief, audit entries; every menu target; not-found; বাংলা.
>
> **Fixed while verifying.** Any in-place menu action froze the whole screen (the menu stayed modal with pointer events disabled). Child-record writes matched on the child id alone, so a request under one workspace's path could change another workspace's record, and a missing record returned 200 — all now scoped and 404. Raw database errors came back as 500 (now 400/404/409). Malformed dates were silently dropped. A `datetime-local` entry at 21:10 was stored as 02:40 next day (read as UTC). Contradiction review credited the case IO whoever clicked. Missing from the UI despite the plan: edit workspace, edit/remove person, task assignee, recording an officer gap, confirmations on destructive actions, loading and error states per panel. Each action refetched the whole workspace (about seven requests), enough to hit the per-address rate limit within minutes.
>
> **Still open.** The per-address rate limit (see Foundation) still throttles a busy officer; the client now retries throttled reads with backoff but that is mitigation, not a fix. The relationship graph is a radial layout that will crowd beyond a few dozen nodes. Gap kinds are fixed in the schema.

Investigation workspace binding a case to its working material: persons, chronology, evidence links, tasks, recorded contradictions, identified gaps.

**Delivered**
- Migration `000026`, 8 tables. ~27 routes under `/api/v1/investigation`.
- Workspace CRUD, persons, chronology with sources, contradictions with officer review, tasks, evidence links, assembled case brief.
- Counts and progress computed in SQL on read, so they cannot drift from the child rows.
- Frontend: `lib/api/investigation.ts`, `hooks/use-investigation.ts`, both screens on live data.

**Gaps work without AI.** Six deterministic rules over the record — no evidence attached, witness statements missing, empty chronology, unexplained interval over 60 minutes, no accused recorded. Each states what it examined and **closes itself** when its condition is resolved. Completing a task linked to a rule-derived gap re-runs the rules rather than forcing the gap closed, so a file cannot claim completeness while material is genuinely missing.

Every table carries `origin` (`officer | supervisor | derived | ai`) so AI suggestions drop in later without reshaping anything.

**Completed in the closing pass**
- **Officer directory** — `GET /investigation/officers` returns assignable officers with rank, badge, station and how many cases each already carries, so load is visible at the moment of assignment. Wired into both the create and reassign dialogs; no more pasting UUIDs.
- **Evidence attachment** — the evidence register is readable and writable, and evidence attaches to a workspace from a searchable picker that marks what is already attached. Attaching closes the no-evidence gap.
- **Relationship graph** — `GET /investigation/{id}/links` assembles persons, their phones and vehicles, attached evidence and the places events occurred. Every node is a stored row and every edge a reference between two of them; nothing is inferred. Replaces the hardcoded diagram that stood in for it.
- **Vehicle and location counts** — both were specified and neither was ever computed; `vehicles` returned 0 always and `locations` did not exist. Vehicles is now a distinct count across persons, and migration `000028` added `location` to chronology entries so distinct places can be counted.
- Timeline entries carry where they happened; tasks can be deleted; evidence can be detached.

**Fixed while closing it** — a finding well outside this phase. Migration `000017` had attached a `capture_changes_*` trigger to ten core tables (`firs`, `cases`, `evidence`, `warrants`, `bail`, `alerts`, `personnel`, `vehicles`, `cyber_crimes`, `citizen_complaints`) writing to `entity_change_log` with a foreign key to an unpopulated `jurisdictions` table. **Every insert into any of those tables failed**, and the handlers swallowed the cause. That is why the core tables were empty. Migration `000029` removes the triggers — federated change capture has no meaning on a single central server. FIR creation additionally failed on an empty `priority` enum, now defaulted.

---

### Phase 02 — Evidence & Chain of Custody · `DONE`

Tamper-evident digital evidence: registration, hashing, custody transfers, access audit, verification, court view.

**What makes it tamper-evident** — cryptography and database constraints, not procedure:
- The SHA-256 is taken **as the bytes stream into storage**. A client cannot supply a digest and nothing accepts one if offered.
- Verification **re-reads the stored object and recomputes**. It measures the file as it is now, not a value recorded earlier.
- Every custody leg — including the first, written at registration — is signed (HMAC-SHA256, server-held key, payload version 2) over the item, its position, both parties and places, purpose, seal number and state, condition note, the file's digest at that moment, the signer, the exact stored time and **the previous leg's signature**. The chain and court views **re-derive every signature on each read** and report it `valid`, `invalid`, `legacy` or `unsigned`.
- `evidence_custody`, `evidence_access_log` and `evidence_integrity_checks` refuse UPDATE, DELETE and TRUNCATE at the database (migration `000036`).
- Integrity has **three** states. `pending` means no file, or none checked since upload — it never reads as intact.

**Delivered**
- Migrations `000030`, `000031` and `000036`. Routes under `/api/v1/custody`, including `POST /custody` for signed registration.
- **Storage abstraction** (`internal/storage`) with filesystem and MinIO backends behind one interface; atomic writes; object keys cannot escape the storage root.
- Register linked to a case or FIR, list and search, stats, attach once, authenticated download with the digest in `X-Evidence-SHA256`, verify, verification history, signed transfer to a receiving officer or place, chain view, access log, court verification, forensic request raised from an item.
- A file can be attached **once**; a correction is a new item with its own history.
- Custody screens fully bilingual (English and বাংলা).

**Verified — 2026-09-14, through the UI.** A 42-check browser run registers an item against a case, attaches a file (the server's SHA-256 equals a digest computed independently), verifies it, downloads it (the copy's digest is recomputed on arrival and matches), records a broken-seal transfer without a note (refused with the reason shown) and a signed transfer to an officer, then **appends one byte to the stored file outside the platform**: the next verification shows `Mismatch` with both digests, both checks are kept, the page raises the mismatch, and the access log carries the failed check alongside the download (with its purpose), transfer and upload. The court view reports every signature valid and discloses no case or FIR number. A forensic request is raised from the item. Every register row action is followed. A constable's transfer is refused with the API's message and writes nothing. The screens are checked in বাংলা with no English custody labels left. Separately at the API: eight concurrent transfers give eight distinct positions, and UPDATE/DELETE on the custody tables is refused. Signature checking was tested against in-memory copies of a stored chain — an altered purpose, a backdated leg, a flipped seal, a removed leg, swapped legs and a different key are each reported `invalid` — without touching the append-only rows.

**Fixed while completing it**
- **Signatures could not be verified at all.** The HMAC covered a timestamp taken in Go while `signed_at` stored the database's later `NOW()`, the payload left out position, previous holder and seal, and no code ever checked a signature. Now signed over the stored moment and verified on every read.
- **"Append-only" was not enforced.** Nothing stopped UPDATE or DELETE on the custody, access-log or integrity tables; triggers added.
- **Concurrent transfers could take the same position** — MAX(sequence) was read without a lock. Transfers now lock the item and run in one transaction; unique index on position.
- **An unsigned transfer route remained** — `POST /evidence/:id/transfer` wrote custody legs without a signature. Removed. Registration's first leg was also unsigned and written outside the item's transaction; `POST /custody` now signs it atomically.
- **Download never worked in a browser** — a plain link cannot carry the bearer token. Now an authenticated download that records a purpose and checks the copy's digest.
- Unknown ids on transfer and verify returned 500 with raw error text; now 404, validation errors are 400 with the reason, and 500 causes are logged.
- Register search matched only the evidence number although it promised description and seal; a control labelled "Attach to a court submission" only navigated; the module description claimed a blockchain-anchored ledger. All corrected.

**Not included, by design** — external anchoring. `blockchain_anchor_tx` exists and nothing writes it. The record is tamper-evident without it; anchoring adds third-party corroboration and is a later layer.

**Open**
- The custody signature is an HMAC proving the platform recorded the leg and that it has not been altered. It is not a personal digital signature bound to an officer's own key pair, which would need a PKI the deployment does not have — and anyone holding `CUSTODY_SIGNING_KEY` could re-sign an altered leg. Key custody is therefore part of the evidence guarantee.
- Legs written before 2026-09-14 are shown as `legacy`: signed, but not re-derivable. They are not reported as intact.
- Court verification is available to any signed-in officer; a court or forensic role belongs with the RBAC model.
- The evidence `status` column is not maintained by custody movements (registration writes `COLLECTED`; transfers to a lab or court do not change it).
- The immutable-audit-style tables still accept INSERT from any database session; the guarantee against forged *new* legs is the signature, not the database.

### Phase 03 — CCTV & Video Intelligence · `DONE` (functional layer)

An intelligence layer over cameras Kolkata Police already operates, not another CCTV installation. Functional without AI; detection, natural-language search and multi-camera tracking remain the AI layer and are not built or imitated.

**Delivered**
- Migration `000040`: `cameras`, `camera_health_checks`, `video_events`, `video_access_log`. `/api/v1/video/*` (routes and role floors documented in the Phase 03 block of `main.go`).
- **Camera register** — code, location, coordinates, station, operating agency (KP/KMC/Traffic/Private/Other), stream type RTSP/ONVIF/NVR or none, host/port/path. Stream credentials are encrypted at rest (AES-256-GCM, key from `CCTV_CREDENTIAL_KEY`, falling back to the JWT secret) and never returned — responses carry only `hasCredentials`. Edits keep credentials unless replaced or cleared; changing the stream target discards earlier health results. Decommissioning requires a reason.
- **Health is only ever measured.** A reachability check opens a TCP connection to the configured host and port with a 3 s timeout and stores every result. The register shows `REACHABLE`, `UNREACHABLE`, `UNCHECKED` or `NO_STREAM` — never "online" without a successful check, and the screen states that reachable means the port answered, not that video flows.
- **No simulated video.** Live playback needs a media gateway that is not deployed; the feed panel says so and names the configured stream target.
- **Operator-raised events** — type, severity, time on the footage, description; `origin = officer`. Triage is confirm or dismiss by an officer **other than the raiser** (service check and table constraint); dismissal needs a note. Only a confirmed event links to a FIR or case, picked from the registers; linking a case carries its FIR.
- **Privacy governance from the first release** — retention class per camera (SHORT 7 d, STANDARD 30 d, EXTENDED 90 d) inherited by events, with `retain_until` computed from the footage time; an event whose footage is already past retention is refused. Linked events move to `EVIDENTIAL` with no expiry and cannot be downgraded. Masking flag on cameras and events. Expired events are excluded from search, return 410 on open or triage, and are deleted by an audited purge that never touches evidential events.
- **Purpose-logged access** — searching or opening event records requires a stated purpose (≥ 10 characters). The purpose, filters, result count, officer and address are written to an append-only `video_access_log` (update/delete refused by trigger) and to the audit trail *before* results are released. Searches never refetch on their own, so every logged search is one an officer ran.
- **Role floors** — view register and raise events: any officer; reachability check, search and open events: ASI; triage and linking: SI; register/edit cameras and change retention or masking: SHO; decommission, purge and read the purpose log: DSP. The UI shows controls by the same floors and explains a refusal rather than hiding the tab.
- Every change audited with its actor: `camera_registered`, `camera_updated`, `camera_health_checked`, `camera_decommissioned`, `video_event_raised|viewed|confirmed|dismissed|linked|retention_updated`, `video_events_searched|purged`.
- Frontend: `lib/api/video.ts`, `hooks/use-video.ts`, `components/video/*`, rewritten `app/video-intelligence/page.tsx`; bilingual (`video.*` in both dictionaries). Mock cameras and events removed from `lib/platform/mock.ts`; the dashboard tile reads events awaiting triage from the API.
- **Live streaming through the Edge Agent** (branch `feat/cctv-streaming`, migration `000076`) — the Live Feed Portal / KMCP design on this camera register, not a second camera table. How to connect a camera, R2 and the Vercel variables: [`docs/CONNECT-A-CAMERA.md`](docs/CONNECT-A-CAMERA.md).
  - Enabling streaming (at registration or later, SHO) issues a per-camera ingest key and a token shown once (SHA-256 stored, constant-time compare); rotate revokes the old token; turning off or decommissioning revokes it, ends viewing sessions and purges the stored feed.
  - `PUT/POST/DELETE /api/edge/ingest/:key/*file` for the agent, outside `/api/v1`, excluded from the rate limiter, flat HLS names only (.m3u8/.ts/.m4s/.mp4/.aac/.vtt), 20 MB cap. `GET` for the camera's own token or an officer inside a session. Playlists `no-store` at every layer (CDN-/Vercel-CDN-Cache-Control), segments immutable (`private`).
  - Live video store `MEDIA_BACKEND` separate from evidence: `r2`/`s3`/`minio` (short timeouts, path-style) or `fs`; `fs` refused on Vercel, `database` refused for live video.
  - Liveness judged from the stored playlist on each read with bounded concurrency: ONLINE / CONNECTING / STOPPED (end marker, or not rewritten for 30 s — the agent's `omit_endlist` means a dead agent never writes one) / OFFLINE.
  - Viewing is purpose-logged once per session (`VIEW_LIVE` in `video_access_log` with the cameras; audit `video_live_viewing_started|ended`), ASI floor, SHO for masking-flagged cameras, 15 min idle / 8 h hard expiry.
  - Frontend: live wall (lazy tiles that play only on screen with the tab visible, pause all, focused view), live panel on the camera sheet and the GIS camera selection, one-time Edge Agent settings dialog with copy buttons, rotate/turn off, honest states, bilingual (`liveVideo.*`). hls.js with Authorization via `xhrSetup`, live-edge re-seek, token renewal on 401.
  - Verified: unit tests; API probe 95/95 (token hash/rotation, 401/400/413, traversal, content types and cache headers, rank and masking floors, one log entry per session, liveness transitions, delete, disable/decommission purge, audit, 200 ingest requests/min with no 429 while the limiter throttles other paths); runtime refusals (fs on Vercel, inherited and explicit database); a real ffmpeg HLS PUT publish with the agent's arguments; the **unmodified Edge Agent** (`live-feed-only`) publishing an RTSP source into NPDMS with only the three values; browser 35/35 — two feeds decoding at 1280×720 on the wall near the live edge, focus view with one decoder, pause releasing players, sheet playback as ASI, constable refused, Edge Agent settings and copy, rotation, Bengali, map panel playback.

**Verified** — an API probe of 102 checks covering rules and refusals (duplicate code, malformed stream host, half coordinates, credentials never in responses and not stored in plaintext, real reachable and unreachable checks, stream change invalidating health, retention inheritance and past-retention refusal, purpose required and recorded, self-triage refused in the service and by the table, link before confirmation refused, evidential downgrade refused, expiry hiding and 410, purge keeping evidential events, append-only log, role floors, decommissioned-camera refusals, audit coverage with actors). A browser run of 39 checks drove every form as SHO, constable and DSP — register with credentials, check reachability, details and edit, unreachable camera, API validation shown, constable rank limits, raise events, purpose refusal and review, open, confirm, link to a case, dismiss with and without note, own-event triage blocked, purpose log, decommission, purge — with each write confirmed in Postgres and the Bengali screen checked.

**Fixed in shared code while verifying** (frontend)
- `CountUp` animated only once per mount, so every stat tile whose value arrives from the API after mount stayed at its first number — usually 0 — on every screen that uses `StatTile`.
- `ActionMenu` prevented the dropdown from closing when an action ran. The modal menu stayed mounted under the dialog the action opened, and its `pointer-events: none` on `<body>` outlived the dialog, so the whole page stopped responding after any kebab action that opened a dialog.

**Open**
- Snapshot and clip export are not built; masking is still not applied to media (live viewing of masking-flagged cameras is limited to SHO and above instead).
- Live streaming: R2 not yet provisioned, so production still reads "Live video storage is not configured"; recording/retention of live video beyond the agent's rolling window is not built; no alert when an ONLINE camera goes STOPPED.
- Reachability is on demand; a scheduled sweep of all active cameras is not yet running.
- ONVIF device discovery and NVR channel enumeration are not implemented — connection details are entered by hand.
- Purge is manual (DSP); an automatic retention job is a later step.
- `go vet ./...` still fails on the pre-existing `testutil/fixtures.go`.

---

### Phase 04 — Missing & Vulnerable Persons · `DONE`

Local operational workflow for missing persons, functional without AI. National matching through NCRB/UNIFY and ICJS remains the authoritative channel; the platform is **not integrated** with it and says so on screen — entering details there is a checklist step the officer performs.

**Delivered**
- Migration `000042`. Extends `missing_person_reports` rather than duplicating it, so a report a family files on the citizen portal and one registered at the station are one record: `REPORTED` → `SEARCHING` → `FOUND` (traced, returned) or `CLOSED` (deceased, other), closure note required. Legacy rows normalised without inventing outcomes. ~17 routes under `/api/v1/missing-persons`.
- **Vulnerability flags** — child, elderly, disability, mental health, trafficking risk. Any flag raises priority; a child or trafficking risk is critical; anyone under 18 is always flagged a child. Held by table constraints as well as the service.
- **First-24-hours checklist** — created when the search starts, each step due a stated number of hours later, two extra steps for a child (FIR, SJPU/CWC). Completion records officer and time; overdue is computed on read and drives a filter and a stat.
- **Sighting register** — any officer records; ASI and above verify or reject (rejection needs a reason), never the recording officer (service and constraint). Coordinates paired, no future sightings, none before last seen.
- **Movement reconstruction** — the last-seen point followed by verified sightings only, in time order, with elapsed minutes.
- **Family communication log** — officer, direction, channel, family member, summary, time. Allowed after closure.
- **Lookout linking** — a MISSING notice is issued on the existing lookout register from the report and linked, rather than keeping a second notice list; closing the report resolves it.
- **Children's records** — identifying details masked in lists for ranks below SI unless the officer registered, took up or is assigned to the report; the record itself returns 403 to them. Every view of a child's record, and every denied attempt, is audited.
- Frontend: `lib/api/missing-persons.ts`, `hooks/use-missing-persons.ts`, both screens on live data, bilingual labels. Fake appearance-match scores, confidence meters, a camera search that searched nothing and a family update that sent nothing were removed.

**Verified** — 92-check API probe (rules and negative paths) and a browser run as ASI, inspector and constable (30 checks: registration, checklist, sightings verified and rejected by a second officer, movement, family contact, edit, lookout, closure, masking, restricted and not-found states, Bengali), with every write confirmed in Postgres.

**Fixed in shared code while building it**
- Citizen portal MIS numbers counted rows; now the shared counter. Its tracking query used `SELECT *`, which the new columns would break. Its tracking route could never match `MIS/YYYY/NNNNN` (a path parameter cannot hold slashes). Public tracking returned the full record — reporter's phone, a child's description — to anyone guessing a sequential number; it now returns progress only. A citizen report for a minor is flagged as a child.
- **Every kebab-menu action that opened a dialog froze the page** after the dialog closed (`pointer-events: none` left on `<body>` by stacked Radix modal layers). Fixed in `components/platform/actions.tsx`; affects all modules.

**Photographs, city-wide board and search map** (branch `feat/mpboard`, not yet merged)

Requested by Kolkata Police: a face thumbnail on every row, the family's photographs first when a report is opened, one list of every report from every station that each station checks as soon as it is lodged, and a map of where the person was seen.

- **Migration `000070`.**
  - `missing_person_photos`: storage key, SHA-256, type, width and height, source (`FAMILY`, `FRIEND`, `REPORTING_PERSON`, `OFFICER`, `CCTV_STILL`, `OTHER`), who provided it and their relationship (both required), consent flag and note, date taken, quality note, uploader.
  - Photos are retired with a reason, never deleted; a trigger refuses `DELETE`.
  - A partial unique index allows only one active primary per report. The first photo becomes primary. When the primary is retired, the next family photo is promoted.
  - `missing_person_reports` gains a paired last-seen coordinate.
  - `storage_objects` backs the database storage backend.
- **Photograph handling.** Only JPEG, PNG and WebP are accepted, decided by magic bytes; the limit is 8 MB and the pixel count is capped.
  - Location metadata is removed before storage without re-encoding the picture. The EXIF GPS directory is emptied in place, keeping orientation and other fields; XMP is dropped. This applies to JPEG APP1, PNG `eXIf`/`iTXt` and WebP `EXIF`/`XMP`. The photo records what was removed.
  - A 160 px JPEG thumbnail is drawn on the server and stored as its own object.
  - Bytes are served with `Cache-Control: private, no-cache` and an ETag, so every view is authorised again and a repeat view gets a 304.
  - ASI and above upload and choose the primary. SI and above retire.
  - Upload, set-primary, retire, every view of a child's photograph and every refused view are audited with the actor.
- **Broadcast visibility (decision).** While a report is open, its purpose is to have the person found.
  - Every officer may see the broadcast view: the primary photograph, name, age, sex, physical description (height, complexion, marks, clothing), last-seen place and time, and the reporting station. This applies to a child's report too.
  - Everything else keeps the rule above: the informant and phone, family contacts, circumstances, other photographs, sightings, the map and the full record.
  - The existing list masking is unchanged.
  - A constable who opens a child's report gets the broadcast card instead of a bare 403.
  - When the report closes, the broadcast ends and the primary photograph falls back under the child-record rule.
- **City-wide board** (`GET /missing-persons/board`, screen `/missing-persons/board`). It lists every open report from every station, newest first. Each report shows its thumbnail, flags, last seen, reporting station, time since lodged and each station's latest check. Any station that has not checked is listed.
  - **Updates arrive by polling every 20 seconds, not streaming.** The staging API runs on Vercel functions, which do not hold long-lived connections, so Server-Sent Events would be cut off and would tie up an instance per viewer.
  - A banner announces reports lodged since the board was opened. The polling cost is 3 requests a minute per open board.
- **Station checks (migration `000071`, append-only).** ASI and above at any station record one of three results for their station:
  - "checked — no match here";
  - "possible match" (details required);
  - "sighting". This creates an ordinary sighting in the same transaction, and a different officer must still verify it.
  - The check is open for a child's report too, because it is part of the broadcast.
- **Automatic alert.** Registering a report, or taking up a citizen-filed one, raises a `BOLO` alert linked to the report through the new `alerts.resource_type`/`resource_id` columns. The alerts screen links back to the report.
  - Scope is `DISTRICT` (Kolkata Police), or `STATE` with priority 1 for a critical report (child or trafficking risk).
  - Closing the report expires the alert.
  - Phase 04 has no reopen workflow, so there is no reopen alert.
- **Search map** (`GET /missing-persons/:id/map`, the Map tab). It is restricted like the full record. Only stored coordinates are plotted.
  - It shows the last-seen point, sightings (verified, awaiting verification and rejected, each drawn differently), active cameras as context, and a time-ordered route through the last-seen point, verified sightings and confirmed camera matches. A slider walks the route.
  - Selecting a point shows who reported it, when, the source and the review status. For camera matches it also shows similarity, model version and the reviewer.
  - Camera matches are read from the face recognition layer's `face_match_candidates` (`PENDING` and `CONFIRMED`), guarded by `to_regclass`. Until that table exists the map says camera matching is not connected. A confirmed match that has already become a sighting is not drawn twice.
  - The last-seen and sighting forms now use the gazetteer location picker. SI and above can place a missing last-seen pin on older reports.
- **Storage backend `database`** (`STORAGE_BACKEND=database`). Objects live in Postgres (`storage_objects`), streamed through SHA-256, with a per-object cap: `STORAGE_DB_MAX_OBJECT_BYTES`, 8 MB by default.
  - This is the persistent option on Vercel today.
  - **Anything over the cap is refused with 413.** The message says object storage (MinIO/S3) must be configured. On this backend, body-worn camera recordings and most footage cannot be stored.
  - To use it on staging: apply `000070` to Neon first (the API refuses to start on this backend without the table), then set `STORAGE_BACKEND=database`. `vercel.json` still says `filesystem` and was not changed.
- **Verified.**
  - API probe: 79 checks, run on the database backend.
    - Uploads: valid JPEG, PNG and WebP; text renamed `.jpg`, GIF, over 8 MB, missing provider or relationship, duplicate and rank all refused.
    - GPS removed in all three formats and still decodable; SHA-256 of the served bytes equals the recorded hash equals `sha256(data)` in Postgres; thumbnail ≤ 160 px.
    - Primary: one active primary, enforced by the database; retirement promotes the next photo; deletion refused.
    - Masking: unchanged for a constable (403 on the child's record, gallery, family contacts and map). The broadcast thumbnail and photograph are served and audited; a non-primary child photo is refused and audited.
    - Board and alerts: the board shows Park Street reports to Jadavpur and omits the informant; checks and their append-only trigger work; alert scope, link and expiry on closure are correct.
    - Sighting, verification by a second officer, and map path.
  - `go test ./internal/storage` database round trip, including refusal over the cap.
  - Browser: 29 checks with two contexts.
    - Park Street registers with a synthetic family photograph. Jadavpur's open board shows the report within 20 seconds, with a banner, no reload and a thumbnail. Jadavpur opens it and sees the photographs first, then records "checked — no match".
    - Jadavpur records a pinned sighting and Park Street verifies it. The map shows the last-seen point, the sighting and the route.
    - The Bengali board renders, and every write is confirmed in Postgres. A further 5 checks cover a constable opening a child's report and seeing the broadcast card.

**Open**
- Appearance matching is the AI layer; the face recognition branch builds it on `missing_person_photos` (migrations `000072`+).
- No reopen workflow exists, so there is no reopen broadcast.
- The board limits itself to the newest 300 open reports.
- The legacy `photo_url` column is unused.
- `inter-agency` screens still read the old mock missing-person list.

---

### Phase 05 — Cybercrime & Financial Fraud · `DONE`

Functional without AI: complaint intake, officer-recorded entities, fraud network, money trail, linked-complaint clusters, freeze requests, recoveries and a loss dashboard — on real data through the API, with an audit trail, driven through the UI in a browser.

**Delivered**
- Migration `000044`. Complaints stay in `cyber_crimes`, gaining the NCRP acknowledgement number, the 1930 helpline reference, `reported_loss_paise`, station and registering officer. The float `financial_loss` column was carried across as paise and removed — **every amount is integer paise**. Complaint numbers moved off `COUNT+1` onto the atomic counters (`000033`).
- New tables: `fraud_entities` (a shared register, one row per normalised phone, UPI ID, bank account + IFSC, wallet, URL or email), `complaint_entities` (which complaint names which entity, in which role), `fraud_transactions`, `freeze_requests`, `fraud_recoveries`. 19 routes under `/api/v1/cyber-crime`, replacing the previous handlers.
- **Entities are recorded by the officer and validated, never extracted.** `internal/fraud` normalises each type — `+91 98300 12345`, `09830012345` and `9830012345` are one phone; UPI IDs and emails are lower-cased; accounts need a valid IFSC — so "two complaints share this UPI ID" is a stored fact. Covered by unit tests.
- **Links are stored references; nothing is inferred.** A complaint's network is the complaint, the entities it names, other complaints naming those entities, and transfers recorded between them. Clusters follow a rule stated on screen: complaints sharing a recorded entity in any role other than the victim's own, transitively. A victim's own account connects nothing.
- **Freeze lifecycle** drafted → sent → acknowledged → frozen or rejected. Recording "sent" is an officer's action with the channel named; the platform does not transmit to a bank and the screen says so. Each stage's columns are required exactly when the stage is, by table constraint.
- **Money rules**, in the service and the schema: a freeze cannot exceed the reported loss; the amount frozen cannot exceed the amount requested; recoveries cannot exceed the loss (the complaint row is locked while checking); the loss cannot be lowered below an open freeze or what has been recovered; transfers only between UPI IDs, accounts and wallets recorded on the complaint; an entity a transfer or freeze depends on cannot be unlinked.
- Loss dashboard (reported, frozen, recovered, freeze requests by stage, complaints linked to another) computed in SQL.
- Frontend: `lib/api/cyber-fraud.ts`, `hooks/use-cyber-fraud.ts`, `/cyber-intelligence` and `/cyber-intelligence/[id]`, bilingual (`fraud.*` in both dictionaries). The legacy `/cyber-crime` screens redirect. Removed the mock complaints, the "mule account" table with velocity scores and AI badges, the fabricated extraction panel and a "Send request" button that only navigated away.

**Found and fixed in existing code**
- `GET /cyber-crime` returned 500 on any row — sqlx scanned `c.*` plus joined columns into a struct that could not hold them. The create succeeded, so complaints could be registered and never listed.
- The digital-evidence and OSINT endpoints on complaints were an unsigned evidence store beside the Phase 02 custody register and were used by no screen; removed. Complaint evidence belongs in `/custody`.
- The module description advertised "mule account detection", which is the AI layer and not built.

**Verified** — 72 API checks including every rule and illegal lifecycle step; table constraints exercised directly with illegal writes; a browser run as an officer: invalid phone and IFSC refused with the reason shown, three entities, two transfers, a freeze refused above the loss then taken through every stage to ₹64,000 frozen, a ₹21,000 recovery, a second complaint sharing the UPI ID appearing in the network and cluster (and the victim's own account not linking), Bengali tabs, a constable shown no write controls, every write confirmed in Postgres with an audited actor.

**Open**
- Backward freeze transitions (e.g. frozen → sent) are prevented by the stage-guarded update, not by a table constraint — stage order is not expressible as a row check.
- `financial_trails`, `graph_nodes`, `graph_edges` and `osint_reports` remain in the schema, unused by Phase 05; the generic `/graph` routes still carry model-style `risk_score` and `confidence` columns. Retire or repurpose with the AI layer.
- Legacy free-text columns on `cyber_crimes` (`suspect_*`, `bank_account_involved`, `crypto_wallet_address`, `transaction_ids`) are superseded by the entity register and no longer written.
- Clustering loads every connecting link into memory; fine at station scale, to be moved into a recursive query before state-wide volumes.
- Mule-account scoring and automatic extraction from complaint text are the AI layer and deliberately absent.

---

### Phase 06 — Traffic Incident & Accident Reconstruction · `DONE` (functional layer)

**Functional without AI.** Incident registration, camera correlation by location and time, ANPR association, signal-phase correlation, collision timeline, draft report for officer approval.

Measured and estimated values must remain visually distinct — a speed derived from camera calibration is not the same kind of fact as a timestamp.

**Delivered**
- Migration `000050`, 8 tables; `/api/v1/traffic-incidents` — register, edit, list with search (reference, location, registration number), report-status and fatal filters, stats, and a single `/:id/workspace` read for the incident screen.
- Attached records, each audited with its officer: vehicles (registration numbers stored normalised, so `WB-06-BC-2210` and `WB06BC2210` are one vehicle), persons with injury severity (drivers and passengers must belong to a vehicle on the same incident), camera footage windows (whether a window covers the incident time is computed on read), plate reads (matched to an involved vehicle on read), signal phases (whether a phase was in effect at the incident time is computed on read), and timeline facts.
- Prior challans for the involved vehicles, read from `traffic_challans` on the normalised registration number.
- **Provenance is structural.** Every timeline fact is `MEASURED`, `OBSERVED` or `ESTIMATED` with its source. An estimate cannot be stored without its method (service check and table constraint); a value range is accepted only as an estimate; units are set by the server from the quantity. The assembled timeline states provenance for every entry — an ANPR read and a controller-log phase are `MEASURED`; a footage review, officer or witness is `OBSERVED` — and the client never infers it. On screen the three differ in shape as well as colour (solid, outlined, dashed with ≈), and an estimated value is shown as a ≈ range with its method, never as a point.
- **Reports** move DRAFT → SUBMITTED → APPROVED, or back via RETURNED with a reason. Submission freezes a snapshot of the stored facts with its SHA-256. Approval needs SI and above and an officer other than the drafter (service and table constraint). An approved report is final (trigger). If the record changes after submission, the report says so rather than silently changing.
- Times on attached records must fall within 24 hours of the incident, which catches a date typed into the wrong year.
- Recording needs ASI and above; reading is open to any officer.
- Frontend: typed client, one workspace query per incident screen, list and incident pages on live data, full বাংলা strings for these screens. The previous screen's mock incidents, trajectory diagram, AI confidence meter and fake "register and correlate" flow are removed.

**Verified** — an API probe covering rules and negative paths (estimate without method, measured range, inverted range, driver without vehicle, vehicle from another incident, ANPR read without camera, footage window reversed or a year off, approval by the drafter, approval below SI, editing a submitted or approved report, a second open report, drift after approval), and a browser run as ASI, SHO and constable driving every form, with writes confirmed in Postgres and the বাংলা labels checked.

**Found while building** — shared, not fixed here:
- **The global rate limit stops an officer mid-entry.** The incident screen as first built refetched each panel separately after every write; within a few edits it reached 100 requests a minute and writes began returning 429. Phase 06 now reads one workspace response, but any module with several panels per screen has the same exposure. Belongs with the rate-limit decision under Core records.
- **Gin binding tags leak validator internals to officers** (`Key: 'X.Latitude' Error:Field validation for 'Latitude' failed on the 'required' tag`). Phase 06 request bodies carry no binding tags and are validated in the service; other modules still use them.
- `next dev` with Turbopack refuses a `node_modules` symlinked from outside the project root; `--webpack` works (relevant only to parallel worktrees).

**Open**
- **Camera references are free identifiers.** They join the Phase 03 camera register when both branches land; correlation by radius and time window then becomes a query against that register.
- **No ANPR data source exists.** Plate reads are officer-entered with their source; an ANPR feed is an integration.
- Signal phases are officer-entered from controller logs; there is no controller integration.
- Trajectory reconstruction, speed from footage and automatic detection are the AI layer and are not built.

---

### Phase 07 — Dispatch & Resource Optimisation · `DONE`

**Functional without AI.** Incident intake, operator classification, distance-ranked dispatch of units from the existing registers, the unit's own progress, stated escalation rules, closure and response analytics.

**Delivered**
- Migration `000052`: `dispatch_incidents`, `dispatch_assignments`, append-only `dispatch_events` (trigger-enforced). Routes under `/api/v1/dispatch`.
- **Intake** from 112, 100, control room, citizen app, walk-in or an officer in the field — any officer may log one. Caller phone validated and normalised; coordinates optional but paired; the call time cannot be in the future. Numbers `INC-YYYY-NNNNN` from the atomic counter.
- **Classification** by an ASI or above: incident type and a severity chosen from a stated four-level scale, each level with its meaning and arrival threshold. Nothing computes a severity. An incident cannot be dispatched until classified (service and table constraint).
- **Units are not a new register.** A dispatchable unit is a fleet vehicle with its allocated driver, or an on-duty officer from personnel. Availability is derived and explained in words: maintenance, reserved, no crew, off duty, on leave, crewing a vehicle, or committed to a named incident.
- **Recommendation** ranks available units by haversine distance from the incident to the vehicle's GPS fix, or its station when there is none, and says which. It is labelled a straight line, never a route or ETA. Units without a position are listed after, unranked. The operator chooses.
- **Assignment is concurrency-safe.** The incident and unit rows are locked, the officer (as unit or driver) is serialised with an advisory lock, and partial unique indexes allow one active commitment per vehicle and per officer. A unit that became unavailable is refused with a 409 naming the reason — never silently double-booked.
- **The unit's own steps** — acknowledge, on scene, clear — are recorded by the assigned officer, or by an ASI or above on their behalf (a radio report). Only the next step is accepted; the table constrains status against timestamps and requires times to run forwards. A unit en route can be stood down with a reason; a unit on scene must be cleared. Incident status is derived from its assignments. Closure requires every unit cleared or stood down, an outcome, and a note or the linked FIR.
- **Escalation rules** are configuration served at `GET /dispatch/policy` and shown on screen: unacknowledged after 2, 5 and 8 minutes escalates to levels 1–3; acknowledged but not on scene within the severity's threshold (10/15/20/30 min) alerts the supervisor. Applied every 30 seconds and before every read, idempotently, each recorded as an event attributed to the rule and audited. Operators can also escalate manually with a reason.
- **Response analytics** (SI and above) computed in Postgres with `percentile_cont`: call→dispatch, dispatch→acknowledge and acknowledge→scene, median and 90th percentile with sample counts, per station and overall, for a chosen period.
- Frontend: `lib/api/dispatch.ts`, `hooks/use-dispatch.ts`, `app/dispatch` rewritten on live data with bilingual strings (`dispatch.en.ts`, `dispatch.bn.ts`). Removed the mock queue and units, the fake ETAs, the "eta-engine" and "incident-classifier" badges with invented confidence, "suggested" severity, and the escalate dialog that recorded nothing. Mutations refetch only the open incident and mark board lists stale, so normal use stays inside the API rate limit.

**Verified** — API probe 76/76 against local Postgres with the rate limiter on: intake validation, classification gates, ranking by distance and position source, illegal transitions, stand-down and closure rules, role floors, and a concurrent race assigning one vehicle to two incidents (exactly one 201 and one 409, one active row). Escalations checked by moving timestamps back: three ladder events once each, one arrival alert, append-only log refuses updates. Analytics matched hand-written SQL for incident count, samples, median and 90th percentile. Browser run 25/25 as admin and constable: intake → classify → ranked recommendation → dispatch → acknowledge → on scene → clear → close with the API's validation message surfaced, full timeline in order, unacknowledged unit escalated to level 3 by rule, stand-down, analytics count against SQL, Bengali labels, and the constable seeing no operator controls. Writes confirmed in psql throughout.

**Open**
- **No notification channel.** Escalations are recorded and shown on the board; they are not sent by SMS, radio or push, and the screen says so.
- **Vehicle positions are whatever the fleet register holds.** There is no live AVL feed; without a GPS fix the station location is used and labelled as such.
- **Unit acknowledgement from the field** needs a mobile client; today the officer uses the web screen or a supervisor records it on their behalf.
- **Station-scoped visibility** (an SHO seeing only their own station's board) belongs with the RBAC model; all authenticated officers see every incident.
- Severity scoring and ETA prediction are the AI layer.

---

### Phase 08 — Station Workload · `DONE`

Renamed from "Station Workload & Performance": it measures operational load, never individual officer performance, and the screen now says so.

**No new tables and no migration.** Read-only aggregation over FIRs, cases, forensic requests, court hearings, warrants, bail, evidence, lookouts, investigation tasks, weapon issuances and personnel. Existing indexes cover the joins; every endpoint answers in under 10 ms on the local data.

**Delivered** — `/api/v1/workload/*`, SHO and above:
- **Station dashboard** (`/summary`): 19 open-item counts in four groups, each returned with a one-line definition of exactly what it counts and a link to the module holding the records. Counts that represent something past a recorded date are marked.
- **Backlog by age band** (`/backlog`): open items in four pipeline stages — under investigation, awaiting forensic results, charge-sheeted or in court, open tasks — banded 0–30, 31–90, 91–180 and over 180 days, with the date each stage's age is counted from stated.
- **Bottleneck**: a deterministic rule, returned with its text — the stage holding the most items older than 90 days; ties go to more items older than 180 days, then the larger open total; no stage is named when nothing is older than 90 days.
- **Officer load** (`/officers`): per officer, FIRs and cases as IO, cases in court, workspaces, open and overdue tasks, pending forensics on their cases, hearings in the next 14 days. Listed by name, never ranked or scored; no single "load score". Every view is written to the audit trail with the actor.
- **Time limits** (`/sla`), each with its source and what it cannot see: accused in custody without a charge-sheet against BNSS s.187(3) (listed from day 45, banded past 60 and past 90); forensic requests past the expected date recorded on the request (no turnaround norm assumed); active warrants past validity; tasks past due; weapons not returned on time.
- **Trends** (`/trends`): weekly or monthly counts of dated events — FIRs and cases registered, forensic requests submitted and completed, warrants executed, investigations closed, bail decided.
- **Station comparison** (`/stations`, DSP and above): per station open investigations with age bands, over 90 days, pending forensics, in court, open tasks, roster and available strength, and open investigations per available officer as a station-level ratio.
- **Scope rule**, enforced by the service: an SHO sees their own station only (another station or a district is 403); DSP and above see any station, a district rollup or all stations. Filters by station, district and an opened-between date range.

**Frontend** — `/workload` rebuilt on the API: scope and date filters, overview, backlog, time limits, officer load (with a sheet per officer), trends and, for DSP and above, stations. The fabricated station table, "Executive brief", AI bottleneck notes and the 30-day forecast with confidence scores were removed. English and বাংলা labels for every figure.

**Verified** — 40 API checks against a dataset seeded through the API at Kasba PS and backdated in Postgres: every count equals an independent SQL count and the change the seed makes; age bands, the date window, the bottleneck tie-break, each time-limit rule, officer figures, trend totals, the audit entry, and 403/400 for scope and input violations. 25 browser checks: every figure on each tab equals the API response, filters and row actions work, Bengali labels render, and an SHO sees only their own station with no comparison tab.

**Fixed while building it** — `CaseRepository.AddAccused` accepted `arrestDate` and never wrote it, so no accused recorded through the API carried the date the BNSS custody period runs from.

**Open**
- **Custody period start.** BNSS s.187(3) runs from the first remand, which is not recorded, so the arrest date is used; and the punishment class of the sections is not recorded, so a case past 60 days must be checked against the 90-day period by the officer. The screen states both.
- **FIR and case closures are not trended** — the records hold a status but not the date it was reached. A status-change timestamp on FIRs and cases would make them trendable.
- **Stations have no `district_id`** in the local data, so district rollups group by the `stations.district` text.
- **Definitions and rule texts come from the API in English**; labels are bilingual.
- `/dashboard` still reads station figures from `lib/platform/mock.ts`; it could now use `/workload/summary`.

Forecasting remains the AI layer.

---

### Phase 09 — Citizen Complaint & Grievance · `DONE`

Functional without AI, on the existing `citizen_complaints` / `complaint_updates` register rather than a second one.

**Delivered**
- Migration `000054`: channel with provenance (only the web portal receives directly; counter needs the recording officer; mobile, WhatsApp, email and call centre need the source reference they arrived under — enforced by constraint), text script, priority, categorisation, duplicate links, anonymous access-code hash, generated normalised phone, `simple` full-text `tsvector` with GIN index. New `complaint_routings` (every jurisdiction change with its reason) and `complaint_responses` (drafted by one officer, reviewed by another). Routing and status history are append-only by trigger; a reviewed response cannot be changed.
- `/api/v1/complaints` (officer register): list with search, status, category, channel, script, unrouted, overdue and open filters; stats; intake; categorise (acknowledges); route to a station with unit, optional officer and reason; status transitions from a fixed table; internal notes; duplicate candidates and linking; FIR link; draft and review responses. ASI to act, SI to reject or approve. Every change and every view of a complainant's record audited with the actor.
- **Bengali and mixed-script search**: prefix `tsquery` over the `simple` configuration, so বাংলা words match as written (`লরি রাস্` finds `লরি রাস্তা`); tsquery syntax characters stripped from input. Tracking-number and phone-digit search alongside.
- **Duplicates by stated rule only**: same normalised mobile within 30 days, or same source reference on the same channel. The officer chooses; linking closes the duplicate, refuses self-links, chains and cycles, and locks both rows in id order.
- **Response approval**: a citizen sees a response only after an SI or above other than the drafter approves it (service check and table constraint). A complaint cannot be resolved without an approved response.
- **SLA ageing computed on read** against stated service standards — acknowledge within 24 hours, resolve within 30 days — returned by the API and shown on screen as configuration, not statutory limits.
- Screens: `/grievance` register, detail sheet with every action as a real dialog, and the public `/citizen` portal (file, track, FIR status), all bilingual. Fixture grievances, the fake AI category/jurisdiction suggestions with confidence scores, the fake voice recorder, and the portal's Karnataka branding, invented statistics and mocked tracking removed. The portal's missing-person form, which submitted nothing, now directs to 112 / the station (Phase 04 owns missing-person reporting).

**Security of the public routes**
- Tracking needs the tracking number **and** the complainant's mobile number, or the one-time access code given to an anonymous complainant (only its SHA-256 is stored). Unknown numbers and wrong second factors give identical responses, so tracking numbers cannot be enumerated. Every attempt is audited with IP and user agent.
- The public view carries status, public-safe bilingual history, approved responses, station name and rejection reason only — no officer names, internal notes, contact details, ids or the other complaint in a duplicate link.
- Body limits (64 KB submit, 4 KB track → 413), per-IP limits (10 submissions/hour, 20 tracking or FIR-status lookups/15 min), strict input bounds and phone/email validation; decoder and validator messages are logged, never shown.
- Tracking moved to `POST` (tracking numbers carry `/`; phone numbers stay out of URL logs). The old `GET /public/complaints/:trackingNumber` never matched and returned the full record including contact details.

**Fixed along the way**
- `GET /public/fir-status` could never succeed: the response struct lacked `db` tags, so every lookup scanned nothing and answered "not found". Also matched the phone as stored text only. Now `POST`, rate-limited, phone normalised on both sides.
- Grievance numbers moved to the atomic counters (were `COUNT+1` with errors discarded). Complaint numbers now `CMP-YYYY-NNNNN` from the counters.
- The old citizen-portal complaint handlers (list, update, assign, resolve, reject, convert-to-FIR) discarded errors, returned `null` lists and exposed `SELECT c.*`; they are replaced by the register above and removed.

**Verified** — API probe 63/63 (public security, Bengali search, every rule and negative path) and a browser run 21/21 with a citizen, an ASI and an Inspector as the second approving officer: file in mixed script → Bengali search → categorise → route → internal note → draft → counter complaint from the same phone linked as duplicate → approve by second officer → resolve → citizen tracks (wrong phone refused; view carries no officer or internal data) → anonymous tracking by access code → public FIR status → Bengali labels on portal and register. Writes and audit confirmed in Postgres.

**Open**
- Only web and counter receive directly; WhatsApp, email, mobile app, call centre and NCRP remain officer-entered with a reference — no integration.
- Grievances against officers (`grievances`) and FIR-copy requests are still submission-only: no officer workflow or public tracking yet.
- The login form rejects usernames shorter than three characters, so the demo `si` and `hc` accounts cannot sign in through the UI.
- Automatic categorisation, jurisdiction suggestion, similarity-based duplicate detection and voice-to-text are the AI layer.

---

### Phase 10 — Public Safety Risk & Hotspots · `DONE`

Deliberately **not** "crime prediction". This scores places, never people, and produces no watchlist.

**Delivered** — migration `000056`, routes under `/api/v1/risk` (SHO and above; below DSP an officer sees only their own station; weights change at SP and above).
- **Documented weighted sum.** `score = Σ weight × count` over five factors: FIRs in the period, high/critical-priority FIRs, night-time FIRs (20:00–05:59), alerts issued for the station, and the increase over the previous period of equal length. Every response carries each factor's count, weight and contribution, the formula text and what each figure counts.
- **Versioned weights** (`risk_weight_sets`). A change is a new version with a required reason (≥10 characters); the audit entry records the actor, the old→new values and the reason. Weight history is visible to every viewer.
- **Areas.** Station jurisdictions, plus officer-defined **beats** (named centre and radius). FIRs store no incident coordinates, so an FIR counts towards a beat only when an officer places it there (`risk_fir_placements`, one beat per FIR, same-station enforced by trigger). Each beat shows how many of its station's FIRs in the period are placed; alerts are marked as not attributable to beats.
- **Period and shift filters** with the previous equal-length period compared per area.
- **Patrol recommendations** — a stated rule: top N areas by score, excluding zero, ties by FIR count then name; the reasoning names the largest contributing factor with its arithmetic.
- **Deployment simulation** — arithmetic only: coverage = scores of areas allocated at least one unit ÷ total score. No predicted change in incidents.
- **Screen** rebuilt on the API: factor table per area, recommendations, simulation, weights and history, beats, FIR placement, map from stored coordinates only, Bengali labels. Removed the mock areas, the "projected score / response time" simulator with a confidence badge, and the AI badges; module marked live and not AI-assisted.

**Verified**
- API, 49 checks: station and beat scores equal hand-computed weighted sums from seeded FIRs with known dates, times and priorities (e.g. Bhowanipore 5 FIRs, 3 serious, 2 night, 1 alert, +3 → 25; after doubling the FIR weight → 30); each contribution equals weight × count; FIRs just outside the window excluded; night shift excludes FIRs without a time; placement across stations refused; beat with placements cannot be removed; SHO confined to own station; SHO and DSP refused weight changes; short reason, missing factors and unchanged weights refused; weight change audited with actor, reason and old→new. **Every one of 34 Phase 10 responses was scanned for the seeded complainant name and phone and for person-related field names — none present.**
- Browser, 21 checks as admin, SHO and constable: screen factors, contributions and score equal the API's and the hand sum; FIR tile equals a SQL count; simulation shows 25.00 of 29.00 covered (86.21%); weight change through the dialog, rule message shown for a short reason, score updates; beat created, FIR placed and beat score shown, then both removed through the UI; Bengali renders; SHO sees one station and weight history only; constable sees the restricted state.

**Open**
- FIRs have no incident coordinates, so beat figures depend on officers placing FIRs. Capturing coordinates at FIR registration would remove that step.
- Factor labels, descriptions and rule texts come from the API in English; screen labels are bilingual.
- Dispatch calls and traffic incidents (Phases 07 and 06) are not yet factors; add them once those branches merge, as new factors with their own weights.
- Beats are circles; polygon boundaries would need PostGIS or stored vertex lists.

### Phase 11 — Police Knowledge Assistant · `DONE` (functional layer)

**Functional without AI.** A document repository for standing orders, SOPs, circulars, manuals, notifications and statute text; keyword and metadata search in English and বাংলা; classification enforced in SQL; versioning by supersession; procedure checklists officers follow for a case or FIR.

**Delivered** — migration `000058` (`000059` reserved, unused), 16 routes under `/api/v1/knowledge`, screens `/knowledge`, `/knowledge/[id]`, `/knowledge/checklists/[id]`.
- **Repository.** Files stream through the Phase 02 storage abstraction; the SHA-256 is taken as the bytes are stored and sent back as `X-Document-SHA256` on download, where the screen recomputes it. Numbers `KD-YYYY-NNNNN` from the shared counter. Upload limit 50 MB.
- **Text, honestly.** `pdftotext` reads PDFs with a text layer; plain text is read as UTF-8. A PDF with no text layer or an image is recorded `OCR_UNAVAILABLE` with the note "OCR is not available on this server (tesseract is not installed)… findable by its metadata only" — no text is invented. The note is shown on the document.
- **Search.** `simple` tsvector (no Bengali configuration ships with Postgres) weighted title › description › file text, plus substring matching, plus pg_trgm word similarity **only for queries with non-Latin letters** — it catches Bengali inflections (গ্রেফতারের → গ্রেফতার) without matching near-miss English reference numbers. Each hit says where it matched (title, details, document text) and shows a highlighted passage. The edge database must use a UTF-8 locale or Bengali tokenisation degrades.
- **Visibility.** `PUBLIC` every officer, `RESTRICTED` ASI+, `CONFIDENTIAL` SHO+, `SECRET` SP+ — the floor is a generated column and every query is bounded by it, so hidden documents are never listed, counted or returned; a direct request answers 404, not 403. An officer cannot file or reclassify above their own clearance. Opening any non-public document and every download is audited.
- **Versioning.** Filing a new version inserts it and marks the old `SUPERSEDED` in one locked transaction (a unique `supersedes_id` stops a double supersede); the old version stays readable with a link forward. Withdrawal (SP+) and reclassification (SP+) need a reason and are audited.
- **Checklists.** SI+ create a checklist from an effective document and a section reference; ASI+ follow it for a case or FIR and tick steps with an optional note. Ticks are append-only (database trigger) and audited; a checklist whose source is later superseded says so.
- **Removed from the screen:** the sample answers, confidence meter, suggested questions, training quiz, fake "Queue for OCR" and the mock document list. The module is marked live and not AI-assisted; its description no longer claims source-cited answers.

**Verified.** API probe 60/60 — visibility by rank including counts and 404s, supersession and its conflicts, reclassification, withdrawal, English phrase inside a PDF, Bengali title, Bengali body phrase and inflected form, scanned PDF found by reference but not by image text, checklist rules, append-only ticks, and every audit event with its actor. Browser run 34/34 as inspector and constable: filing through the form, searches, download digest check, new version, checklist creation, run and tick, Bengali screens, and a constable meeting "Document not found" for a restricted document. Writes confirmed in Postgres.

**Open**
- **OCR.** Needs tesseract (with Bengali data) on the edge server; the pipeline stage records its absence today but the OCR call itself is not wired.
- **Semantic search and answering** are the AI layer. Non-negotiable when it lands: cite the document relied on, and refuse when no filed document covers the question.
- **Statute text** is filed as documents; there is no section-level index of BNS/BNSS/BSA yet.
- **Login form** rejects usernames under three characters, so the seeded `si` account cannot sign in through the UI (outside this phase).

### Phase 12 — Case File & Court Readiness · `DONE`

A court file assembled over an investigation workspace (Phase 01) and the signed evidence register (Phase 02). It owns no persons, evidence or chronology: every entry references a stored record — the investigation's FIR, an evidence item, a forensic request — or a document uploaded through the evidence storage with its SHA-256 taken as the bytes are stored.

**Delivered**
- Migration `000060` (case files, entries, charges, evidence support, witness facts, versions, packs). 27 routes under `/api/v1/case-files`. Reading needs ASI, building the file SI, approval Inspector.
- **File index** with serial numbers derived on read from the order the court expects — FIR, statements, seizure lists, forensic reports, custody records, police report, other — and frozen into a pack's manifest. Statements name the witness, the BNSS provision and the date. Removal needs a reason and the document stays in earlier versions.
- **Evidence matrix** — each charged section against the evidence supporting it, with each item's integrity state and custody-chain verdict read live from Phase 02 (every signature re-derived on each read). A support row's composite key into `workspace_evidence` means only the investigation's own evidence can be linked.
- **Witness matrix** — witnesses, victims and complainants against the facts they speak to, citing their statements and the provision each was recorded under.
- **Completeness** — ten deterministic rules in the Phase 01 gap style, evaluated on every read, so none can be dismissed while its condition holds: FIR filed; a statement for every witness; seizure list; forensic requests completed with reports filed; no evidence with a failed integrity check; stored files verified (advisory); every custody leg verifiable; legacy legs (advisory); every charge supported by evidence; the parts of the BNSS s.193(3) report the record can show. Each states what it examined.
- **Version history** — every successful change increments the version and writes a full snapshot in the same transaction; versions refuse UPDATE and DELETE.
- **Submission packs** — submitting freezes an ordered manifest (documents with file or record digests, charges with supporting evidence, evidence integrity and chain state, open findings) stored as the exact text hashed, so `manifest_sha256` can be recomputed by anyone. Open findings do not stop submission but a pack frozen with blocking findings cannot be approved. Approver must differ from submitter (service and table constraint); approving a pack older than the file is refused; one undecided submission per file; frozen and decided packs refuse change at the database. A change after approval marks the file stale.
- Every write is scoped to the file in the path — a child id from another file is a 404, and references outside the file's investigation are refused with the reason. Every change is audited with its actor.
- Frontend: typed client, scoped hooks, list and detail screens with every form, credentialed download that recomputes the digest on arrival, browser-side manifest hash check; English and বাংলা. The mock documents, readiness percentages and AI governance notice are removed.

**Verified — 2026-09-15.** API probe 113/113, including: cross-file writes (linking another file's evidence, removing or downloading through another file's path, another investigation's FIR or witness); every completeness rule opening and closing itself as its condition changes; a stored evidence file tampered with outside the platform — the next verification is broken, the matrix shows it live, the rule opens as blocking, the pack submitted in that state records it in its manifest and cannot be approved, and restoring and re-verifying closes the rule; self-approval refused; approval of a stale pack refused; direct UPDATE/DELETE on packs and versions refused by the database; the manifest hash recomputed independently. Browser run 23/23 as Inspector (build and submit), SHO (approve, second session) and constable (refused): open from an investigation, file a FIR record, a statement (first refused without its provision, with the API's reason shown), seizure list, police report and forensic report; download with the received digest matching; add a charge and link evidence showing live integrity and a verified chain; record a witness fact citing a statement; every blocking rule closes; view version 1 empty; submit and see the manifest hash match in the browser; approval by the second officer confirmed in Postgres; a later removal marks the approved file stale; screens checked in বাংলা.

**Open**
- Arrest and custody particulars required by s.193(3) are not recorded in the workspace, so that rule cannot check them and says so.
- Completeness details are returned in English; rule titles and descriptions are bilingual.
- Consistency checking across documents is the AI layer and is not built.
- The login form rejects usernames shorter than three characters, so the demo `si` and `hc` accounts cannot sign in through the UI (shared code, not changed here).

---

### Phase 13 — Body-Worn Camera Evidence · `DONE` (functional layer, no AI)

Body-worn cameras on the Phase 02 evidence register — not a second evidence store.

**Delivered**
- Migration `000062`: `bwc_devices`, `bwc_assignments`, `bwc_readings`, `bwc_recordings`, `bwc_access_log`. 20 routes under `/api/v1/bodycam`.
- **Device register** with service status (in service, charging, faulty, retired — a reason required for the last two; a camera that is issued can be marked faulty but not retired or put on charge). Numbers `BWC-YYYY-NNNNN` from the atomic counters.
- **Shift issue and return**, like the armoury ledger: one open assignment per camera and one camera per officer, both enforced by unique partial indexes under a row lock — a simultaneous double issue gives one 201 and one 409. Overdue returns computed on read.
- **Battery and storage** are readings reported by a dock or an officer with the moment observed, never polled telemetry. A reading older than 12 hours is shown stale; future and week-old readings are refused.
- **Docking**: a recording is uploaded against the open assignment in the path (the wearing officer, or ASI and above), streamed through the Phase 02 storage abstraction and hashed as it arrives — the client never supplies a digest. Upload against a returned assignment is 409; footage must fall inside the shift; 2 GiB limit. Numbers `BWR-YYYY-NNNNN`.
- **Retention classes.** The Phase 02 register only accepts items linked to a case or FIR, and most footage never becomes evidence, so docked footage is held here as non-evidential for 31 days. **Linking** (SI and above) to an FIR or case — optionally a dispatch incident — re-hashes the stored bytes, refuses if they no longer match the docking digest, then registers the recording in the Phase 02 register: signed first custody leg, the same file attached (its register digest is required to equal the docking digest), and Phase 02 verification, chain and access log from then on. A concurrent double link gives one 200 and one 409 and exactly one evidence item.
- **Purge** of footage past non-evidential retention is a DSP action with a reason; it removes the bytes and keeps the record. Evidence is never purged — refused by the service and by table constraints.
- **Access control.** Listing recordings needs ASI; opening or downloading footage needs a stated purpose (10+ characters), written to an append-only access log before anything is returned; refusals are logged as DENIED. Downloads of evidential footage go through the Phase 02 register, which logs its own access with the purpose. The access log is DSP and above.
- **Integrity held by the database**: a recording's digest, officer, device and time span cannot be rewritten, an evidential recording cannot return to non-evidential, a purge cannot be undone, recordings are never deleted, and the access log is append-only.
- Every change audited with its actor. Domain errors map to 400/404/409/413 with officer-readable messages; 500s log the cause.
- Frontend: `/bodycam` rebuilt on the API — camera register, camera sheet (issue, dock, return, readings, status, ledger), recordings with purpose-gated opening, download with the SHA-256 recomputed on arrival and the result shown persistently, verification, linking through the case/FIR register picker, custody chain with live signature status, access log and purge. Bengali throughout. Mock cameras, the fake transcript, AI tags and the chain-anchor badge removed; the module is marked live and not AI-assisted.

**Verified**: API probe 80/80 — including the double-issue and double-link races, upload against a closed assignment, viewing without a purpose, purge of linked footage refused, a linked recording verifying intact through the register with a valid custody-leg signature, and the database refusing to rewrite a digest or delete the access log. Browser run 20/20 as SHO, ASI, Inspector and DSP with every write confirmed in Postgres. Found through the browser: the download digest header was not exposed by CORS, so every download read as a mismatch — fixed.

**Open**
- **Playback** needs a media gateway that is not deployed; the honest option is download with a purpose.
- **No dock or device integration**: readings and uploads are officer actions; there is no vendor dock API.
- **Custody between docking and linking** is the docking digest, the access log and the audit trail, not a signed custody leg; the signed chain starts at linking, because the register requires a case or FIR.
- Transcription, speaker separation and event detection are the AI layer and are not built.
- The login form rejects usernames under three characters (`si`, `hc`) — a shared defect already recorded under Phase 09.

---

### Phase 14 — Malkhana / Seized Property · `DONE` (functional, no blockchain)

Seized property against an FIR or case: where it is kept, the seal it is kept under, every movement out and back, and disposal under a court order recorded in the system. The custody ledger is the hash-chained audit trail plus an append-only per-item history; anchoring remains the later layer and nothing claims it.

**Delivered**
- Migration `000066`: `malkhana_locations`, `property_items`, `property_seal_checks`, `property_movements`, `property_events`. 20 routes under `/api/v1/malkhana`. Numbers `MLK-YYYY-NNNNN` from the shared counter. Money in paise.
- Rules held by the database as well as the service: one open movement per item (unique partial index), nothing leaves while the seal is recorded broken, disposal only against a court order for the same case, narcotics destruction only with a DSP-rank witness, seal checks and history append-only, returned movements and disposed items immutable — all enforced by triggers and constraints, so a write that bypasses the service is still refused.
- Seal verification: a seal read under a different number than recorded is not intact whatever was ticked. A break marks the item, raises a station alert, and blocks movement until an SHO records a reason (≥15 characters) and reseals.
- Movements: forensic (CFSL/FSL), court production against a hearing in the court diary, inter-station, interim custody on court order. Handover requires the current seal number. Overdue and review-due (180 days, a stated operational setting) are computed on read.
- Printable label whose QR encodes only the property number and a verify URL (no personal data), generated server-side. Printable forwarding letter assembled from the register for every item sent under the same memo to the same laboratory that day.
- Officers below DSP work only with their own station; other stations' items read as not found.
- Screens: register with server-side search/filters/pagination and dashboard, item page with every action as a real dialog surfacing the API's message, label and letter pages, verify redirect. Bengali throughout via `malkhana.bn.ts`.

**Verified** — API probe 102/102 including negatives, the concurrent-movement race (exactly one 201 of six) and direct-SQL rule bypasses refused; browser run 27/27 as SHO and constable with every write confirmed in Postgres.

**Open**
- Court production links a hearing but the court diary does not yet show the exhibit list.
- The movement ledger is not HMAC-signed like Phase 02 custody legs; it relies on immutability triggers and the audit chain.
- No link yet from a Phase 02 evidence item to its property record from the custody screen (the reverse link exists).
- Seed the Kolkata demo dataset with malkhana locations and items.

---

### Statute library and incident location · `DONE` (branch `feat/fir-location-statutes`, not yet merged)

Reported on `/fir/new`: location suggestions and the map did not work, and only a hand-typed list of about 30 BNS sections was available.

**Statute library (migration `000068`).** It is loaded by default: the tables `legal_acts`, `legal_sections` and `legal_correspondence` are shipped as reference data. Every row comes from an official published text, retrieved 2026-09-15; nothing was typed from memory.

| Act | Sections | Coverage | Source |
|---|---|---|---|
| BNS, 2023 | 358 (1–358) | complete | India Code section records; headings checked against the Gazette text published by MHA; classification of 288 BNS sections from the BNSS First Schedule, Part I |
| BNSS, 2023 | 531 (1–531) | complete | India Code, checked against the Gazette text (MHA) |
| BSA, 2023 | 170 (1–170) | complete | India Code, checked against the Gazette text (MHA) |
| IPC, 1860 | 575 (1–511 plus 64 lettered sections, 21 omitted or repealed) | complete; repealed from 1 July 2024 | India Code consolidated text A1860-45, 17 heading typos corrected from the enacted text |
| IT Act 2000 · NDPS Act 1985 · Arms Act 1959 · POCSO Act 2012 · Dowry Prohibition Act 1961 · MV Act 1988 | 125 · 129 · 48 · 47 · 13 · 257 | complete | India Code section records |
| IPC ↔ BNS correspondence | 533 rows | complete | BPR&D correspondence table |

- **Not sourced:** the Calcutta Police Act, 1866 and the Calcutta Suburban Police Act, 1866. India Code holds no pre-1947 Bengal Acts, the Kolkata Police and WB Police sites returned 503, and the WB legislative sites did not connect. Only unofficial copies were found. SP and above can add these Acts in Settings once an official text is obtained.
- **Checked:** 37 spot-checks against the source text all pass, including BNS 103, 303, 318, 64 and 111, and IPC 302→BNS 103(1), 379→303(2), 420→318(4) and 498A→85 and 86.
- **Protection:** triggers make built-in rows read-only.
- **API** (`/api/v1/legal`):
  - Any officer can search sections by number, heading or citation (such as `IPC 420`) and see the equivalents in the other code, and can look up the correspondence in either direction.
  - SP and above can add Acts, and add, correct or retire custom sections; each change needs a reason and is audited.
  - An attempt to change a built-in row returns 409.
- **Frontend:**
  - The section picker searches on the server. For an IPC section it offers the BNS equivalent, and it warns when an IPC section is cited for an incident on or after 1 July 2024.
  - The hand-typed lists in `wb.ts` are gone.
  - Settings → Legal sections is available in English and Bengali.

**Incident location (migration `000069`).**
- **Cause:** commit `f94cd97` replaced the old LocationPicker, which geocoded through public Nominatim and centred on Bangalore, with a plain text box. `e9b5bd2` then deleted it as unused. FIRs had no coordinate columns.
- **Storage:** `firs` now has `incident_latitude` and `incident_longitude`. They are stored as a pair or not at all and must fall inside West Bengal (21.4–27.3 N, 85.8–89.9 E); both the database and the API enforce this.
- **Suggestions:** they come only from `gazetteer_places`, 2,910 Kolkata localities, roads, landmarks, police, rail and metro stations and PIN codes. This is an offline OpenStreetMap extract (© OpenStreetMap contributors, ODbL), so an address is never sent outside the platform.
- **Map:** the picker uses Leaflet with OSM tiles. Picking a suggestion moves the map; clicking or dragging sets the pin, and with the location field empty the nearest place name is filled in. The FIR detail page shows the pin.
- **Adding places:** Settings → Map places lets SP and above add a missing place, with the addition audited.
- **Verified:** an API probe covering search, correspondence, admin writes, 409 and 403 refusals, and rejection of half or out-of-state coordinates; and a 21-check browser run as SI and as admin with writes confirmed in Postgres.

### Vehicle detection and ANPR (AI layer A4) · `DONE` (branch `feat/anpr`, not yet merged; runs on the edge server, not on Vercel)

Kolkata Police asked where vehicle detection was. It had been deferred with the AI layer (workstream A4 in the plan); this delivers the vehicle and plate part of A4. Speed estimation, natural-language search and appearance matching remain planned.

**Models and licences** (details, digests and the licence review in `services/ml/vehicle_detection/MODELS.md`)
- Vehicle detection: **YOLOX-s** (COCO), Megvii release 0.1.1rc0 — Apache-2.0.
- Plate localisation and reading: **PaddleOCR PP-OCRv4** mobile detection and recognition, ONNX files from the pinned `rapidocr_onnxruntime` 1.4.4 wheel — Apache-2.0.
- Runtime: ONNX Runtime (MIT), OpenCV headless (Apache-2.0), FastAPI (MIT). CPU only, no cloud APIs; the running service downloads nothing.
- Rejected: Ultralytics (AGPL-3.0); the "MIT" open-image-models YOLOv9 plate detector, whose checkpoints show it was trained with the GPL-3.0 yolov9 code; fast-plate-ocr (MIT, but 62% on WB plates against 94%).

**What it detects and reads**
- Classes: car, motorcycle, bus, truck, bicycle. **No class for auto-rickshaw, e-rickshaw, cycle-rickshaw, taxi or LCV** — autos come out as truck or car or are missed. **Colour is not estimated.** Both are stated by the service and on screen.
- Plates: standard (`WB 06 AX 3304`, `WB 19 6695`), BH series and the old Bengal three-letter series (`WBC 1844`), one- and two-line. Normalised with the Phase 06 rule; OCR confusions corrected only where the format fixes the character type, reported with the raw text. Only format-valid reads are kept, which is what keeps shop signs out. Confidence per character.

**Measured** (CPU, 2 threads)
- Synthetic WB plate crops, held-out set of 240: **97.5%** exact (small plates 92.5%, two-line 87.5%). Upper bound — synthetic plates are clean.
- Synthetic plates placed on vehicles in real Kolkata photographs, 198 scenes: **83.8%** found and read; 96% when the plate is ≥150 px wide, 70% under 90 px; simulated night 69%, angled 80%.
- Real licensed photographs (Wikimedia Commons, attribution recorded): 5 of 7 legible plates read correctly, 1 missed, 1 unverifiable; no false plates in 14 photos.
- Detection, manual review of 91 detections in 8 photographs: precision 97.8% (vehicle present), 91.2% with the right class; recall 78.1%.
- Speed: ~46 ms detection, ~0.3–0.5 s for a full 1280 px frame with plate reading.
- Not measured on real night, rain or motion-blurred footage: none was available with a usable licence.

**Delivered**
- `services/ml/vehicle_detection` (FastAPI, stateless): `/health` with model versions, licences and digests; `/v1/analyse/image`; `/v1/analyse/video` sampling frames. Models pinned by SHA-256 (`fetch_models.py`); a missing or altered file makes the service answer 503, never mock output. Dockerfile for the edge server. Plate rules unit-tested; evaluation scripts in `eval/`.
- Migrations `000074`/`000075`: module switch (starts **off**), analyses, frames (object key + SHA-256), vehicle detections, plate reads (confidence, per-character confidence, model version, box), append-only purpose log, vehicle watchlist, watchlist hits, and ANPR provenance columns on the Phase 06 plate-read register.
- API `/api/v1/anpr` (15 routes): status; switch (DSP, reason, audited); submit a still or footage (ASI, purpose required and logged); camera snapshot ingestion; open an analysis with a purpose; frames served only to an officer who submitted or opened the analysis within 12 hours; purpose-logged plate search by full or partial plate, time window, camera and place (radius around a point); watchlist (SI adds/removes with reason and expiry, at most a year; active STOLEN_VEHICLE lookouts included automatically from their registration detail or subject); hit queue and review (SI, never the submitter; dismissal needs a note); reads map; purpose log (DSP). `POST /traffic-incidents/:id/plate-reads/from-anpr` attaches a read with its model version and confidence; the collision timeline shows it as **observed, AI-assisted**, never measured.
- Storage: stills are kept (they are the frame); **footage is never stored whole** — its SHA-256 and size are recorded and only sampled frames with output are kept. On the `database` storage backend (8 MB per object) files over the cap are refused before analysis with a clear message.
- `cmd/anpr-snapshot`: pulls frames from an RTSP URL with ffmpeg and posts them as snapshots; credentials come from the environment and never appear in logs.
- Frontend `/vehicle-detection` (nav: Surveillance, live, AI-assisted, ASI+): service and switch state, upload with purpose, results with frame, boxes, plate reads and per-character confidence, recent analyses opened with a purpose, hit review with frame view, plate search, watchlist, map of reads and hits at camera locations, purpose log. The traffic incident page gains "Attach ANPR read". English and Bengali.

**Safeguards held**
- Detections and reads are stored and shown as machine output with confidence, model version and source frame.
- A watchlist match is a PENDING hit; only confirmation by an officer other than the submitter raises an alert (BOLO, station scope, 24 h) and, for a lookout, records an unverified sighting at the camera's coordinates — in one transaction. Enforced in the service and by table constraints.
- Every submission, snapshot, search and opening is written to an append-only purpose log before results are released; every change is audited with its actor.
- When the service URL is unset or the service is down, status says "not connected", analysis returns 503 `service_not_connected`, nothing is stored and nothing is fabricated; watchlist, search over stored reads and hit review keep working.

**Verified — 2026-09-15.** API probe 61/61 across seven runs: switch off refuses analysis, DSP floor, purpose required, constable refused, detections and reads stored with model versions, frame SHA-256 equal to the upload, lookout match creates a PENDING hit, frame refused before opening with a purpose, partial plate and place/time search, uploader's confirmation refused, dismissal needs a note, confirmation raises an alert and a sighting with the camera's coordinates, double review refused, manual watchlist entry matches, map counts, attach to a traffic incident (window enforced, no duplicates, timeline OBSERVED), footage analysed with only frames stored, service down and URL unset both give "not connected" with nothing stored, 8 MB database-backend refusal, audit entries with actors, purpose log append-only in the database, RTSP CLI against a stream and a refused connection. Browser run 18/18: ASI uploads a still with a WB plate on a stolen-vehicle lookout and sees detection, read and pending hit; confirm disabled for the uploader; SI opens the frame with a purpose and confirms; alert on `/alerts`; sighting on the lookout; map point at the camera; manual watchlist entry; Bengali; attach to a traffic incident; not-connected state with watchlist still working.

**To run it live**
- **Hosting:** the detection service must run on the Kolkata Police edge server (or any on-premises host the API can reach); set `ML_VEHICLE_DETECTION_URL` on the API. The Vercel staging API has no ML service, so the module shows "not connected" there. A DSP must switch the module on.
- **Hardware:** CPU is enough for uploads and a few cameras at one frame every few seconds (4 cores ≈ 4–8 frames/s, ~1 GB RAM for the service). Continuous reading of many junction cameras needs a GPU box (NVIDIA T4 class or an edge accelerator) or one CPU node per few cameras.
- **Cameras:** RTSP/ONVIF access and credentials from the camera owners (KP, KMC, Traffic), network reachability from the edge server, cameras registered with coordinates, and plate-capable placement (plates ≥ ~150 px wide, IR illumination at night). Run `anpr-snapshot` per camera under a service officer account.
- **Before operational use:** a labelled set of real Kolkata junction footage (day, night, rain) to measure accuracy on the cameras that will be used, and fine-tuning for autos/e-rickshaws and Indian plates if the numbers warrant it; a legal review of pretrained weights and of retention.

**Open**
- **Retention** is not enforced for analyses and frames yet (Phase 03 has classes and purge for events; ANPR should follow the camera's class).
- Exact plate matching only; no fuzzy matching.
- No auto-rickshaw/e-rickshaw class and no colour; no real night or rain footage in the evaluation.
- The map uses the platform's Leaflet component, which loads OSM tiles and marker images from the internet; an offline tile server is needed for an air-gapped deployment.
- The Dockerfile was written but not built here (the Docker daemon is not running on the build machine); `docker-compose.yml` does not yet include the service.
- Snapshot ingestion via the CLI polls; there is no continuous stream reader or NVR integration.

---

## Later layers

The detailed plan of action for both layers — workstreams, ground rules, the verdict on each prototype in `services/ml`, anchoring options and everything Kolkata Police must provide — is in [`docs/plans/ai-and-anchoring-plan.html`](docs/plans/ai-and-anchoring-plan.html).

Added over phases that already work without them.

### AI enablement · `PLANNED`

Python ML services exist in `services/ml` (FIR classifier, semantic search, OCR, crime prediction, transcription, video analysis) and are currently not deployed.

Non-negotiable rules, already enforced in the UI vocabulary:
- Every AI output carries a confidence figure, its sources, and a model version.
- Nothing AI-generated becomes FIR content, an accusation, an arrest decision, a charge-sheet fact or an evidentiary conclusion without an officer's approval.
- AI findings are investigative leads, never conclusions.
- `origin: "ai"` already exists on every relevant table.

Hardware note: the ML services need real memory and, for video, a GPU. Sizing against the MDC / AI Edge box is an open question.

### Face recognition for missing persons · `BUILT` (branch `feat/facerec`, not yet merged)

**Decision.** Kolkata Police: "We will use the FR system full scale" to match missing persons' faces, from photographs given by family and friends, against CCTV footage, camera snapshots and reported sightings, with hits shown on the search map. This overrides the default in the AI plan, which left face recognition out. Use is limited to missing-person reports. Any other scope needs an authorisation that names it.

**Decision, 2026-09-15 — build now, host later; demo authorisation only for now.**
- **Not connected.** The live Vercel API has no face recognition service. Every face-matching screen and endpoint says "Face recognition service not connected" when `FR_SERVICE_URL` is unset or unreachable. There are no errors and no invented candidates.
- **DEMO authorisation.** A `DEMO` authorisation type exists alongside real orders. It lets face recognition run only on photos flagged as synthetic test images, uploaded through an administrator-only demo path. It refuses to enrol any real report photo, in the service and in a database trigger.
- **Labelling.** Everything produced under DEMO is labelled "Demo — synthetic faces": enrolments, candidates, searches, sightings created from them, the map popup and every audit entry.
- **Real photos.** Enrolling real report photos needs a real `ORDER` authorisation with reference, date and issuing authority.

**Models and licences** (details, hashes and measurements in `services/ml/face_recognition/MODELS.md`)
- **Detection.** YuNet `face_detection_yunet_2023mar.onnx`, MIT. SHA-256 `8f2383e4…52fa4`.
- **Recognition.** SFace `face_recognition_sface_2021dec.onnx`, 128-d, Apache-2.0. SHA-256 `0ba9fbfa…c34e79`.
- **Source.** OpenCV Zoo commit `47534e27`. Both run on the CPU through OpenCV 4.10. The recorded model version is `sface-2021dec+yunet-2023mar`.
- **Not used.** InsightFace/buffalo (non-commercial weights) and AGPL detectors.
- **Licence caveat for counsel.** SFace's upstream describes training on CASIA-WebFace, VGGFace2 and MS-Celeb-1M, which are web-scraped, and MS-Celeb-1M was withdrawn. The licence on the weights does not settle the training-data question.

**Architecture**
- **`services/ml/face_recognition`** (FastAPI, stateless, Dockerfile for the edge server).
  - `/health` reports model versions, hashes and licences. The service refuses to start if a model hash differs.
  - `/v1/enrol` detects, aligns, embeds and scores quality, and rejects a poor photo with a reason: no face, several faces, too small, blurred, turned, tilted, dark or overexposed.
  - `/v1/match` takes a still or video, samples video at 1 frame per second by default, compares every usable face with the gallery the API sends, merges hits per report within 5 s, and returns the frame and face crop with SHA-256.
  - `tools/rtsp_snapshots.py` pulls frames from an RTSP stream and posts them to the API's snapshot endpoint, for when camera access exists. `tools/evaluate.py` produces the measurements. `tests/` holds the unit checks.
- **API** (migrations `000072`, `000073`; `FaceRecognition{Handler,Service,Repository}`, `FRClient`).
  - **`fr_authorisations`** — ORDER or DEMO; reference, authority, date, scope, validity; revocation with a reason.
  - **`ai_module_switches`** — the per-module switch and its config. A trigger refuses to switch it on without an active authorisation covering `MISSING_PERSONS`. Revoking the last authorisation switches it off in the same transaction.
  - **`fr_synthetic_photos`** — the demo path; the synthetic declaration is required.
  - **`face_enrolments`** — `REAL[]` embeddings. An embedding exists only while status is `ENROLLED`: withdrawing, replacing, closing the report or retiring the photo removes it. A trigger enforces that a real photo needs an ORDER and a synthetic photo needs DEMO.
  - **`face_match_searches`** — every submission with its purpose, submitter, media SHA-256, threshold, model and counts, recorded before matching runs. Media is kept only if it produced a candidate and fits the storage backend's per-object cap. On the `database` backend (8 MB) only frames and the hash are kept.
  - **`face_match_candidates`** — the columns agreed with the map. The evidence fields cannot be changed (trigger). Review happens once; the reviewer is never the submitter; a rejection needs a reason; a confirmation needs a sighting.
  - **pgvector is not used.** It is not installed on the edge Postgres, and the gallery (enrolled photos of open reports) is small, so the service compares by brute force.
- **Endpoints**
  - Status, authorisations (SP and above; DEMO administrator only) and settings (switch, threshold 0.30–0.95, sampling rate; SP and above).
  - Enrol per report or photo (ASI and above). Enrolment is also automatic when a primary photo is uploaded or made primary under an ORDER: a hook in the photo service, plus a 2-minute reconciler that also removes stale templates.
  - Withdraw an enrolment (SI and above).
  - Footage or still search (ASI and above, purpose required) and camera snapshots (ASI and above).
  - Per-report candidates and the city-wide review queue.
  - Confirm and reject (ASI and above; not the submitter). Confirming writes a `VERIFIED` `CCTV_REVIEW` sighting in the Phase 04 sightings table, reported by the submitter, verified by the confirmer, at the camera's coordinates and the frame time.
  - Frame, crop and enrolled-face images behind the child-record rule.
  - Service unreachable returns 503 `fr_service_unavailable`; not configured returns 503 `fr_service_not_connected`. The search is recorded as `FAILED` and no candidates are created.
- **Frontend.**
  - A **Face matching** tab on the report (`components/face-recognition/FaceMatchingPanel.tsx`) shows authorisation and service status, photos with enrolment status and rejection reasons, the administrator's synthetic upload, Search footage with purpose and camera, and candidates.
  - Each candidate shows the enrolled face beside the face in the footage, similarity, threshold, model, camera, location, frame time, frame hash and the source frame with its box, plus Confirm and Reject.
  - **Face Match Review** (`/face-recognition/review`) is the city-wide queue.
  - **Settings → Face recognition** records orders and demo authorisations, and holds the switch, threshold, service and models.
  - Strings are in `face-recognition.{en,bn}.ts`. The search map's match popup carries the demo label.

**Thresholds and measured accuracy** (synthetic faces only: SFHQ part 1, MIT; same-identity probes are degraded copies)
- **Default match threshold 0.50.** OpenCV's 0.363 raised 3–9 wrong candidates per face against 500 enrolled photos.
- **Same identity, at 0.50.** Found for 99.9% of 112 px faces, 99.6% at 64 px, 98.0% at 48 px and 92.4% at 36 px. Most 36 px faces (1,325 of 1,497) are below the 36 px detection-box rule and are not compared at all.
- **Different identities.** p99 0.36–0.39, p99.9 0.44–0.49, maximum 0.83 (near-duplicate synthetic identities). At 0.50 the false match rate is 0.02–0.08% per comparison, which is 0.1–0.4 wrong candidates per face per 500 enrolled photos. It grows linearly with the gallery, so a city gallery of 5,000 enrolled photos means roughly 1–4 wrong candidates per clearly visible face. Human review is not optional.
- **Enrolment quality gate.** 499 of 531 accepted (31 turned away, 1 dark).
- **Speed** on an Apple M4, one worker. Enrolment 11 ms per photo. A 12-second 720p clip at 1 frame per second took about 0.4 s end to end, about 30 ms per sampled frame with decoding. One hour of footage is about 2 minutes per worker.
- **Limits, stated plainly.**
  - No real same-person variation was measured: age gap between the family photo and today (worst for children), clothes, expressions.
  - No real CCTV conditions: overhead angles, motion blur, night IR, backlight, masks and helmets.
  - No demographic breakdown.
  - CPU only.
  - The true-match figures are an upper bound. Measure on authorised, labelled Kolkata footage before enabling for real reports.

**Safeguards implemented**
- **Candidates only.** Every result is a `PENDING` candidate. A second officer of ASI rank or above confirms it before it becomes a sighting; the submitter cannot. Rejection needs a reason.
- **Evidence on show.** Similarity, threshold, model version, source frame and frame SHA-256 are stored, shown and immutable.
- **Audited.** Every authorisation, switch change, refusal, synthetic upload, enrolment and rejection, search (with purpose), failure, candidate and review is written to the hash-chained audit trail with the actor. Demo entries carry the demo label.
- **Switch and authorisation.** A per-module switch, off by default. It cannot be switched on without an active authorisation, and it switches off when the last one is revoked. The order's reference, date, authority and scope are shown on every face recognition screen.
- **On premises.** Data never leaves the platform; the service is on premises and calls nothing.
- **Missing persons only.** Only open missing-person reports are matched. Templates of closed reports and retired photos are removed.
- **Children.** The child-record rule applies to candidate lists, images and review.
- **Not connected.** Without the service, nothing runs and the screens say so.

**Verified**
- `pytest`: 7 checks, all passing. Same identity above and different identity below 0.50 on synthetic faces; no-face, blurred, small-face and two-face photos rejected; model hashes and licences reported; a synthetic clip matched only in the seconds the face is on screen.
- **API probe: 54 of 54.**
  - Switching on without authorisation refused. Order below SP rank refused; order without reference, date or authority refused; DEMO recorded by a non-administrator refused.
  - DEMO refuses a real report photo uploaded through the missing-persons flow. Synthetic upload refused for non-administrators and without the declaration.
  - Blurred photo rejected as `BLURRED`. Search without a purpose, or below ASI rank, refused.
  - Candidates carry similarity, model, threshold, camera coordinates and the demo label; the frame is served with its hash. Confirmation by the uploader, or below ASI rank, refused; rejection without a reason refused; a second review refused.
  - Confirmation by another ASI creates a `VERIFIED` sighting at the camera's coordinates. A still-image candidate rejected with a reason.
  - Service down: 503, no candidates, search recorded `FAILED`, status says not reachable.
  - Under an ORDER, a report photo enrols without the demo label.
  - All 15 audit event types present, and every demo entry labelled.
- **Not connected.** API without `FR_SERVICE_URL`: status `configured: false`, search answers 503 `fr_service_not_connected`, the report tab shows the banner and no search form.
- **Browser (Playwright on :3123): 12 of 12.**
  - SP's attempt to switch on is refused, then SP records an order and switches on.
  - The administrator records DEMO, adds two synthetic photos and enrols them: one enrolled, the blurred one rejected with its reason.
  - SI searches a synthetic clip with a camera and purpose. The candidate shows similarity, threshold, model, camera and the demo label, with no Confirm button for the uploader.
  - ASI confirms from the review queue. "Sighting created" appears, the report's sightings list the demo-labelled verified sighting, and Postgres holds `VERIFIED | 22.5645 | CCTV_REVIEW`.

**What live camera integration still needs**
- **Network.** Access from the edge site to the CCTV management system or the cameras, plus stream credentials for the pilot cameras, stored in the Phase 03 register.
- **Snapshot service.** Run `tools/rtsp_snapshots.py`, or an equivalent VMS SDK integration, as a service per camera group under a control-room integration account.
- **Service placement.** Deploy the face recognition service on the edge server's private network with `FR_SERVICE_TOKEN`, and set `FR_SERVICE_URL` on the API.
- **Asynchronous footage searches.** Footage searches are synchronous today. Long footage needs a job queue so an upload does not hold an HTTP request for minutes.
- **Object storage for footage.** MinIO/S3 is needed to keep footage; the database backend keeps only frames.
- **Measurement and rules before live use.**
  - A real ORDER authorisation, the legal opinion and DPIA from the AI plan, and a published retention policy for templates and frames.
  - Measurement on labelled Kolkata footage, day and night, with the threshold set from it.
  - A nodal officer signing off the threshold.

**Hardware at scale** (estimates to confirm with a load test)
- **Footage searches and enrolment.** CPU is adequate. One core handles about 30 sampled 720p frames per second with few faces. 8–16 cores cover a division's footage uploads.
- **Live snapshots.** At one frame every 5 s per camera, that is about 150 cameras per core with light scenes; crowded scenes (stations, ghats) cost 5–10 ms per extra face. For 1,000 cameras, plan 16–32 cores dedicated to face recognition, or one data-centre GPU with a GPU build of the detector and recogniser.
- **Memory.** About 150 MB per worker process.
- **Gallery.** Brute-force matching of 10,000 enrolled photos is well under 1 ms per face.
- **Storage.** Frames (100–500 KB each) for candidates only.

**Open**
- **Map labelling.** The DEMO label on the map relies on `is_demo`, which was added to the missing-persons map query in this branch. Check it on merge.
- **Legal review of SFace training data** (above).
- **Asynchronous footage jobs** and **camera streams**, both of which need access.
- **Real-footage measurement and demographic evaluation.**
- **Neon.** Neon does not yet have `000072`/`000073`. Applying them is harmless without the service: every screen says not connected.

### Blockchain anchoring · `PLANNED`

The hash chain in `audit_logs` is already tamper-evident. This layer batches those hashes and anchors them externally, writing `blockchain_anchor_tx`.

Scope discipline: large files are never stored on-chain — only cryptographic proofs. Applies to Phase 02, 13 and 14.

---

## Changelog

Newest first. One line per completed task.

| Date | What |
|---|---|
| 2026-09-16 | **Live CCTV streaming through the Edge Agent** (branch `feat/cctv-streaming`). Migration `000076`. The Live Feed Portal / KMCP push-ingest design on the Phase 03 register: one-time ingest token (hashed), `/api/edge/ingest` outside `/api/v1` and the rate limiter, separate `MEDIA_BACKEND` (R2; fs refused on Vercel, database refused), playlist-judged liveness with staleness, purpose-logged live viewing sessions (ASI; SHO for masked cameras), live wall, sheet and map players. Unmodified Edge Agent publishes into it. Probe 95/95, browser 35/35. Guide: `docs/CONNECT-A-CAMERA.md`. |
| 2026-09-15 | **Vehicle detection and ANPR** (branch `feat/anpr`, AI layer A4). YOLOX-s and PaddleOCR PP-OCRv4 (Apache-2.0) in a stateless CPU service on the edge server; migrations `000074`/`000075`; `/api/v1/anpr` with purpose-logged submission and search, a watchlist that includes stolen-vehicle lookouts, operator-confirmed hits raising alerts and sightings, camera snapshot CLI and ANPR reads in the traffic plate-read register; `/vehicle-detection` screen in English and Bengali. Measured 97.5% on synthetic WB plate crops, 83.8% on synthetic plates in real scenes, detection precision 97.8%. Shows "not connected" wherever the ML service is absent, including Vercel. |
| 2026-09-15 | **Face recognition for missing persons** (branch `feat/facerec`). Migrations `000072`/`000073`. On-premises service in `services/ml/face_recognition`: YuNet (MIT) and SFace (Apache-2.0) from the OpenCV Zoo, hash-checked. Recorded ORDER and DEMO authorisations; a per-module switch the database will not turn on without one. Enrolment with a quality gate that gives a reason. Purpose-logged footage, still and camera-snapshot searches. Immutable candidates confirmed by a second officer into verified sightings at the camera's coordinates. City-wide review queue and settings. DEMO enrols only synthetic test faces and labels everything it produces; without the service every screen says "not connected". Default threshold 0.50, measured on synthetic faces with limits stated. Probe 54/54, pytest 7/7, browser 12/12. |
| 2026-09-15 | **Missing persons: face photographs, city-wide board, station checks, search map** (branch `feat/mpboard`). Migrations `000070`/`000071`. Photographs checked by magic bytes with GPS removed and a server-drawn thumbnail, one primary per report, retired never deleted. Every open report broadcast to every station (photo, name, age, sex, description, last seen, station) with the rest still restricted; the board polls every 20 seconds because the Vercel API cannot hold streams. Each station records no match, possible match or a sighting. A linked BOLO alert is raised on lodging. The map plots only stored points and reads face-match candidates when that table exists. New `database` storage backend (8 MB cap) makes photographs persist on Vercel. |
| 2026-09-15 | **Statute library and incident location on the FIR form** (branch `feat/fir-location-statutes`). Migrations `000068` and `000069`. The full BNS, BNSS, BSA and IPC, six special Acts and the BPR&D correspondence table load by default from official sources. Picker, settings and audited custom entries added. The FIR form gets gazetteer suggestions and a map pin, and FIRs store coordinates. The Calcutta Police Acts are not included: no official source could be reached. |
| 2026-09-15 | **Phases 03–14 merged and deployed.** Each phase was built in its own worktree and verified there (API probes and browser runs), then merged into the working branches and re-verified together on one integrated API: every phase probe passes on the merged build (Phase 10's fixed-score checks drift with shared data, so its scores were re-checked as weighted sums of the factors the API reports — all consistent). Merging surfaced cross-phase clashes that git merged silently but that did not compile — duplicate helpers (`bind`, `bindJSON`, `trimPtr`, `ErrStationNotFound`, `Viewer`), same-named complaint identifiers in Phases 05 and 09, a restored duplicate lookout handler, mangled dictionary braces and duplicated dead mocks — all resolved. Fast-forwarded to `main`; Vercel production deploys for API and web; Neon brought to migration `000066` in place (162 tables). Login form accepted only usernames of three or more characters, locking out demo `si` and `hc`; fixed. |
| 2026-09-15 | **Kolkata demo dataset; no fabricated records in migrations.** The base schema and migrations no longer insert any records; the Karnataka seed users, Koramangala station and KOR FIRs are gone (`000064` converts existing databases in place, keeping demo usernames and passwords working). Reference data is 17 real Kolkata Police stations across the eight divisions. `scripts/seed-kolkata-demo.py` creates fictional operational records through the API as the recording officer, so numbers, rules, signatures and audit entries are genuine; idempotent via `demo_seed_ledger`. Loaded into the live Neon database. The dashboard now reads live figures; the login page no longer fabricates a user when the API rejects a sign-in. |
| 2026-09-15 | **Phase 01 verified through the UI.** 35 browser checks and a 17-check API probe across every claimed capability. Fixed a screen-freezing menu bug, cross-workspace writes on child records, 500s for bad input, IST timestamps stored as UTC, a misattributed reviewer, and missing edit/remove/assign/record-gap flows. Screens fully bilingual. |
| 2026-09-14 | **Phase 02 completed through the UI.** 42-check browser run covering registration, attach, verify, authenticated download with digest check, signed transfer, the one-byte tamper proof, access log, court view, forensic request, row actions, rank refusal and বাংলা. Found and fixed: custody signatures were unverifiable (signed and stored timestamps differed, payload incomplete, nothing checked them) — now payload v2 chained to the previous leg and re-derived on every read; custody, access-log and integrity tables were not append-only — triggers in `000036`; concurrent transfers could share a position; an unsigned legacy transfer route and unsigned registration leg; download could not work in a browser; 500s for unknown ids. Custody screens translated. |
| 2026-09-14 | **Core records onto the API complete.** Evidence routed to the Phase 02 custody register (signed transfers only) and alerts rewired; the IndexedDB offline layer, its service worker API cache and the dexie/workbox dependencies removed. Found that the Phase 01 and Phase 02 forms could not be filled in a real browser — 37 handlers read an event where the shared input passes a value; fixed and made a compile error. Alerts: the issuer and acknowledging officer are taken from the session (either could be set to another officer's id), a new alert can no longer be created already acknowledged or claiming an image, and every alert audit entry now records its actor. |
| 2026-09-14 | **Audit hash chain no longer forks under concurrent writes.** Appends read the latest hash and inserted without a lock: 60 simultaneous appends produced 9 forked parents and 51 links not matching their predecessor, so the chain would not have verified under real load. Appends now take a transaction-scoped advisory lock before reading the parent and hold it through the insert — 60 concurrent appends, 0 forks, 0 broken links. Failed audit writes were returned to callers that ignore the error; they are now reported server-side. The 51 broken links from the measuring run remain in the local development database, which is immutable by trigger and was not altered. Armoury, lookout and access-log screens on the API. |
| 2026-09-14 | **Armoury, lookout and access-log backends; FIR and cases on the API.** Migration `000034` adds weapons, weapon issuances, lookouts and sightings, with the workflow rules held by the database as well as the service. Sign-in, failed sign-in and sign-out now written to the audit trail with address and user agent, and served as the access log. Verified by 47 checks covering rules as well as happy paths — double issue, rounds over-return, damaged return without a note, self-verification, sightings on resolved notices, role limits. The first run caught a parameter-type ambiguity in the return transaction; the transaction rolled back cleanly, leaving the weapon correctly issued. Case register now searches and filters by status (it read only page and pageSize, so every case picker showed the eight newest cases whatever was typed); court hearings and orders filter by case; case update 404s for an unknown id, stamps `updated_at`, and returns the stored record. FIR and case screens rewired and verified in a browser. |
| 2026-09-14 | **Six modules on the API end to end: warrants, bail, court, forensics, personnel, vehicles.** Typed clients mirroring the Go models, thin hooks with no fallback, server-side filters, pagination and counts, records linked from registers rather than free text. Each driven in a browser against the local API with writes confirmed in Postgres. Removed controls that reported actions which never happened and every fabricated Bangalore/Karnataka record. Query client fixed: 4xx reads were retried three times (`error.status` vs `ApiClientError.code`) and mutations retried once, which re-sends a create that already landed. |
| 2026-09-14 | **Second backend repair: alerts, evidence, FIR, record numbering.** Alerts could neither be created nor listed (NULL scans). Evidence and case numbers were `UnixNano() % 100000` and collided — observed on the tenth evidence item; FIR, warrant and bail numbers were `COUNT+1`, duplicating under concurrency or after a delete. Migration `000033` adds atomic per-scope counters seeded above every issued number; 200 concurrent allocations gave 200 distinct values. FIR numbers used the first three characters of the station UUID (`550/2026/…`) — now the station code (`BHW/2026/00001`), and an unknown station is rejected. The FIR timeline queried columns that do not exist in the immutable audit table and a table that does not exist, and returned an empty list with 200 — now reads the real audit trail. No core repository checked `rows.Err()`, so a failing list query returned an empty page with 200; checks added to all eleven. `/court/stats` returned `activeCases: 143`, a constant — now counted, with day boundaries from the database clock instead of midnight UTC. Bail status changes erased earlier outcome dates — releasing a granted bail lost its approval date. |
| 2026-09-14 | **Core-record backend repaired: warrants, bail, personnel, vehicles, court hearings, forensics.** Probed every action against local Postgres — 8 of 25 calls failed, and reads that passed did so only because tables were empty. Warrant creation failed on a NOT NULL base-schema column the code never wrote (`warrant_type`); migration `000032` consolidates it and `charges` into the columns the code uses. Bail, personnel and hearing creates **wrote the row and then returned 500**, because the read-back scanned NULLs into non-pointer fields — a retry would duplicate the record. Nullable columns and optional joins now `COALESCE`d in bail, personnel, vehicles, forensics and court queries; `hearing_time` (a `TIME`) formatted as text and written via `NULLIF`; `Vehicle.Type` moved from the traffic-challan enum to the police fleet enum the table enforces; `Warrant.ValidUntil` and `CourtHearing.CaseID` made nullable. Now 39/39 calls pass and every list and get reads back a row with only required columns set. Also found the legacy frontend hooks mask failures with demo data — see "Core records onto the API". |
| 2026-09-14 | **Container builds repaired and verified.** Three places pinned Go 1.22 against a `go 1.25` go.mod — `services/api/Dockerfile`, `Dockerfile.dev` and `.github/workflows/ci.yaml`, so CI was broken as well as the images. All three now on 1.25. Two further defects found while verifying: the production Dockerfile hardcoded `GOARCH=amd64`, yielding an image tagged arm64 that carried an x86-64 binary and only ran where emulation existed — now `ARG TARGETARCH` with an amd64 default; and `Dockerfile.dev` installed `cosmtrek/air@latest`, a renamed module whose current release needs Go 1.26, so an unpinned install broke the build — now `air-verse/air@v1.61.7`. Verified by building both images and running the API container against Neon: `/health` 200, `/ready` reports `database: healthy`. |
| 2026-09-14 | Decided the backend repository stays public for now. |
| 2026-09-14 | **Staging API deployed to Vercel** at `https://api-black-pi.vercel.app`, against Neon. Vercel's Go runtime runs a `package main` that listens on `$PORT`, which `main.go` already did, so no application change was needed — `services/api/vercel.json` supplies the build command, region and non-secret environment. Health, readiness and CORS preflight verified; login not yet exercised. |
| 2026-09-14 | **Fixed a CORS defect that would have blocked every write from the deployed frontend.** The web client attaches `X-CSRF-Token` to each state-changing request, but that header was missing from `Access-Control-Allow-Headers`, so cross-origin preflights failed while `GET` kept working — a frontend that looks half alive. Also stopped pairing `Access-Control-Allow-Credentials: true` with a `*` origin, which browsers reject outright; origins now come from `CORS_ALLOWED_ORIGINS`. |
| 2026-09-14 | **Fixed conflicting `NEXT_PUBLIC_API_URL` conventions in the frontend.** Eighteen callers treated it as a base already ending in `/api/v1`; `ui/web/src/lib/upload.ts` treated it as a bare host and appended `/api/v1` itself, so no single deployed value could satisfy both. `upload.ts` now follows the majority. |
| 2026-09-14 | Oracle Always Free dropped as the demo host — 498 MB usable against a ~550 MB profile, wedged twice. `deploy/oracle-free/` retained; it needs an Ampere A1 shape, not the micro. |
| 2026-09-14 | Added `deploy/oracle-free/` — a 1 GB-shape profile for free demo hosting. Possible only because the Phase 02 storage abstraction lets evidence files sit on local disk instead of MinIO. |
| 2026-09-14 | **Phase 02 Evidence & Chain of Custody completed.** Storage abstraction (filesystem default, MinIO optional), streaming SHA-256 on upload, signed custody transfers, append-only access log, verification with full history, court verification view. Migrations `000030`/`000031`. Proven by tampering with a stored file and confirming the check catches it. |
| 2026-09-14 | Phase 01 re-verified before starting Phase 02 — both suites pass, zero audit failures. |
| 2026-09-13 | **Phase 01 completed in full.** Officer directory, evidence attachment with picker, relationship graph from real records, vehicle and location counts (migration `000028`). Found and fixed a defect blocking every insert into ten core tables: federated change-capture triggers with an unsatisfiable foreign key (migration `000029`), plus an FIR priority enum default. |
| 2026-09-13 | Created this plan of action. |
| 2026-09-13 | Edge hosting hardening: local Postgres, `bootstrap-db.sh`, `deploy/edge/`, `backup.sh`; five migration-chain bugs fixed; federation code deleted; IP lookup moved server-side; API contract and OpenAPI published and served at `/openapi.yaml`. |
| 2026-09-13 | Phase 01 Investigation Copilot completed end-to-end — migration `000026`, 8 tables, ~27 routes, React Query hooks, both screens on live data. Fixed three pre-existing platform bugs: every audit write was silently failing, the session middleware locked clients out after 3 requests, and logout never released the session. |
| 2026-09-13 | UI/UX reconstructed for Kolkata Police — design system, bilingual shell, all 14 module screens, no dead controls. |
| 2026-09-13 | Backend split into `Tatu1984/npdms-backend`; Go API repaired from 262 compile errors to a clean build. |
