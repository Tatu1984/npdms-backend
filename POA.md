# Plan of Action

**Kolkata Police Digital Intelligence Platform**
One platform, fourteen phased modules, for Kolkata Police / West Bengal Police / CID / Traffic Police and the West Bengal courts.

| | |
|---|---|
| Canonical location | `npdms-backend/POA.md` — the frontend repo points here |
| Last updated | 2026-09-14 |
| Current focus | Core records onto the API complete — every core module on the API, no client-side stores or offline layer remain. Phase 03 (CCTV & Video Intelligence) is next |

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

- **Evidence uploads do not persist.** Vercel's filesystem is read-only apart from `/tmp`, which is wiped between invocations. `STORAGE_PATH` is set so the service boots, not because uploads survive. Real storage needs S3/R2, or the edge box.
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
| RBAC model, CCTNS/ICJS interfaces, ML hardware sizing | Still open, tracked in Foundation and Later layers above. |

---

## The fourteen phases

Each phase is complete when its workflows run on real data through the API, with an audit trail — **not** when its screens render.

The UI for every phase already exists and is navigable; where a phase is not `DONE`, those screens are driven by fixture data in `ui/web/src/lib/platform/mock.ts` and are a specification of the intended behaviour, not working software.

---

### Phase 01 — AI Investigation Copilot · `DONE` (complete)

> **Correction, 2026-09-14.** The investigation screens were marked done, but none of their text inputs worked in a real browser: the shared `Input` passed a value while these handlers read `e.target.value`, which threw on every keystroke. Earlier verification exercised the API, not the forms. Fixed in the frontend (`79dfa5e`), and the component's type narrowed so the mistake no longer compiles. The same defect affected the Phase 02 custody register and transfer dialogs. From here, a phase is not `DONE` until its forms are driven in a browser.

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

### Phase 03 — CCTV & Video Intelligence · `PLANNED`

An intelligence layer over cameras Kolkata Police already operates, not another CCTV installation.

**Functional without AI.** Camera register, RTSP/ONVIF/NVR integration, operator-raised events, event triage and confirmation, incident association, purpose-logged search. Detection and natural-language search are the AI layer.

**Backend** — new tables for cameras and events; feed integration; event lifecycle; purpose-based search logging.
**Note** — privacy governance (role-based access, retention, masking, purpose logging) is required from the first release, not with the AI layer.

---

### Phase 04 — Missing & Vulnerable Persons · `PLANNED`

**Functional without AI.** Registration, vulnerability flags, first-24-hours checklist, sighting register with officer verification, movement reconstruction from verified sightings, family communication log.

`missing_person_reports` and `suspect_identifications` already exist; sightings need a table.

Appearance matching is the AI layer. National matching via NCRB/UNIFY and ICJS remains the authoritative channel — the platform's contribution is local operational workflow.

---

### Phase 05 — Cybercrime & Financial Fraud · `PLANNED`

**Functional without AI.** Complaint intake, manual entity extraction, fraud network graph, transaction trail, victim clustering, freeze requests, loss dashboard.

`cyber_crimes`, `financial_trails`, `graph_nodes`, `graph_edges`, `osint_reports` exist. Automatic entity extraction and mule-account scoring are the AI layer; the graph and the money trail are not.

---

### Phase 06 — Traffic Incident & Accident Reconstruction · `PLANNED`

**Functional without AI.** Incident registration, camera correlation by location and time, ANPR association, signal-phase correlation, collision timeline, draft report for officer approval.

Measured and estimated values must remain visually distinct — a speed derived from camera calibration is not the same kind of fact as a timestamp.

---

### Phase 07 — Dispatch & Resource Optimisation · `PLANNED`

**Functional without AI.** Incident intake from control room, phone, app, officer; operator classification; unit register with availability; assignment; acknowledgement and escalation timers; response analytics.

Severity scoring and ETA prediction are the AI layer. Distance-based recommendation and the escalation ladder are ordinary logic.

---

### Phase 08 — Station Workload & Performance · `PLANNED`

**Needs no new tables** — aggregation queries over FIRs, cases, forensic requests, court matters and personnel.

**Functional without AI.** Station dashboard, backlog by age band, officer workload, SLA monitoring, bottleneck identification, trend analysis.

Forecasting is the AI layer. Note: this measures operational load, never individual officer performance.

---

### Phase 09 — Citizen Complaint & Grievance · `PLANNED`

**Functional without AI.** Multi-channel intake (web, mobile, WhatsApp, email, call centre, counter), Bengali and mixed-script text, officer categorisation and routing, duplicate linking, status tracking visible to the citizen, response drafting.

`citizen_complaints`, `complaint_updates`, `grievances`, `public_fir_requests` exist.

Automatic categorisation, jurisdiction suggestion, duplicate detection and voice-to-text are the AI layer.

---

### Phase 10 — Public Safety Risk & Hotspots · `PLANNED`

Deliberately **not** "crime prediction". This scores places, never people, and produces no watchlist.

**Functional without AI.** Incident aggregation by area and time, transparent weighted scoring with every factor shown, patrol recommendations, deployment simulation, period comparison.

The scoring is a documented weighted sum, not a model. Every contributing factor and its weight is visible to the officer reading it.

---

### Phase 11 — Police Knowledge Assistant · `PLANNED`

**Functional without AI.** Document repository for SOPs, circulars, manuals and BNS/BNSS/BSA text; OCR of scanned documents; keyword and metadata search; role-based visibility; procedure checklists.

Semantic search and source-cited answering are the AI layer. **Non-negotiable when it lands:** if no authoritative source covers a question, the assistant says so and does not answer.

---

### Phase 12 — Case File & Court Readiness · `PLANNED`

**Functional without AI.** Case file index with serial numbering, evidence matrix, witness matrix, document completeness checks, version history, submission pack assembly, officer and supervisor approval.

Completeness and consistency checking are deterministic rules over the file, the same approach proven in Phase 01. Builds directly on Phase 01 and Phase 02.

---

### Phase 13 — Body-Worn Camera Evidence · `PLANNED`

**Functional without AI.** Device register, assignment, battery and storage monitoring, secure upload on docking, hashing, case association, retention classes, access control, chain of custody.

Transcription, speaker separation and event detection are the AI layer.

---

### Phase 14 — Malkhana / Seized Property · `PLANNED`

**Functional without blockchain.** Property registration, QR identifiers, storage location, seal recording and verification, movement tracking, forensic transfer documents, court production, disposal against a court order, overdue alerts, audit dashboard.

The custody ledger is the existing hash-chained audit trail. Anchoring is the later layer.

---

## Later layers

Added over phases that already work without them.

### AI enablement · `PLANNED`

Python ML services exist in `services/ml` (FIR classifier, semantic search, OCR, crime prediction, transcription, video analysis) and are currently not deployed.

Non-negotiable rules, already enforced in the UI vocabulary:
- Every AI output carries a confidence figure, its sources, and a model version.
- Nothing AI-generated becomes FIR content, an accusation, an arrest decision, a charge-sheet fact or an evidentiary conclusion without an officer's approval.
- AI findings are investigative leads, never conclusions.
- `origin: "ai"` already exists on every relevant table.

Hardware note: the ML services need real memory and, for video, a GPU. Sizing against the MDC / AI Edge box is an open question.

### Blockchain anchoring · `PLANNED`

The hash chain in `audit_logs` is already tamper-evident. This layer batches those hashes and anchors them externally, writing `blockchain_anchor_tx`.

Scope discipline: large files are never stored on-chain — only cryptographic proofs. Applies to Phase 02, 13 and 14.

---

## Changelog

Newest first. One line per completed task.

| Date | What |
|---|---|
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
