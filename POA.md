# Plan of Action

**Kolkata Police Digital Intelligence Platform**
One platform, fourteen phased modules, for Kolkata Police / West Bengal Police / CID / Traffic Police and the West Bengal courts.

| | |
|---|---|
| Canonical location | `npdms-backend/POA.md` — the frontend repo points here |
| Last updated | 2026-09-14 |
| Current focus | Phases 01 and 02 complete; Phase 03 or the core records next |

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
| Free demo/staging deployment | `DONE` | `deploy/oracle-free/` — Oracle Always Free micro shape (1 GB). MinIO dropped for the filesystem storage backend, Postgres and Redis tuned down, per-service memory limits, 2 GB swapfile, and `setup.sh` covering both Oracle Linux and Ubuntu images plus the instance firewall. Demo and rehearsal only; live case data stays on the MDC box. |
| Backup and restore | `DONE` | `scripts/backup.sh` — verifies the dump before pruning, states plainly when evidence files are not included. |
| API contract and versioning | `DONE` | `docs/api/CONTRACT.md`, `docs/api/openapi.yaml`, `docs/api/routes.txt` (all 292 routes). Served live at `GET /openapi.yaml`. |
| Audit trail | `DONE` | Hash-chained append-only `audit_logs`. Every state change appended; failures logged, never discarded. |
| Federation code removed | `DONE` | 683 lines of browser-side vector-clock sync deleted, plus its dead backend twin. Not needed on a single server. |
| IP/OSINT lookup moved server-side | `DONE` | `GET /intel/ip/{ip}` — controlled egress, audited with stated purpose, private addresses classified locally. |
| **Core records onto the API** | `NEXT` | 13 Zustand stores still hold client-side mock data. Now unblocked — creates against the core tables previously failed at the database. See below. |
| RBAC and permissions model | `PLANNED` | Role checks exist per-route; needs a coherent model documented and enforced centrally. |
| Offline / sync layer | `PLANNED` | Deferred by decision. Online-first now; the offline queue is added across modules once workflows settle. |
| CCTNS / ICJS integration | `PLANNED` | The platform consumes authorised data from systems Kolkata Police already operates. Needs their interface specifications. |
| TLS / reverse proxy | `PLANNED` | Required before any traffic crosses a network we do not control. |

### Core records onto the API — `NEXT`

The largest outstanding foundation item. Thirteen stores hold mock data in the browser.

| | Stores | Work |
|---|---|---|
| Endpoint already exists | fir, cases, evidence, warrant, bail, court, forensics, personnel, vehicles, alerts | Pure rewiring — replace ~2,400 lines of client-side mock with API calls |
| No endpoint yet | accesslog (495 lines), armoury (321), lookout (324) | Tables, repository, handlers and routes first |
| Correctly client-side | auth (already API-backed), toast | None |

---

## The fourteen phases

Each phase is complete when its workflows run on real data through the API, with an audit trail — **not** when its screens render.

The UI for every phase already exists and is navigable; where a phase is not `DONE`, those screens are driven by fixture data in `ui/web/src/lib/platform/mock.ts` and are a specification of the intended behaviour, not working software.

---

### Phase 01 — AI Investigation Copilot · `DONE` (complete)

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

**What makes it tamper-evident** — cryptography, not procedure:
- The SHA-256 is taken **as the bytes stream into storage**. A client cannot supply a digest and nothing accepts one if offered.
- Verification **re-reads the stored object and recomputes**. It measures the file as it is now, not a value recorded earlier.
- Each custody transfer is signed (HMAC-SHA256, server-held key) over the item, the parties, the moment and the file's digest at that moment, so a leg cannot be inserted, reordered or backdated without the signature failing.
- Integrity has **three** states. `pending` means no file, or none checked since upload — it never reads as intact.

**Delivered**
- Migrations `000030` (file metadata, signed custody, access log, integrity-check log) and `000031` (column cleanup). 12 routes under `/api/v1/custody`.
- **Storage abstraction** (`internal/storage`) with filesystem and MinIO backends behind one interface. Filesystem is the default: a single edge server already has disk, and one fewer service is one fewer to operate, secure and back up. Writes are atomic (temp file then rename) so an interrupted upload cannot leave a partial file where evidence should be, and object keys cannot escape the storage root.
- Attach, download (digest travels in `X-Evidence-SHA256`), verify, verification history, custody chain, transfer, access log, court verification.
- A file can be attached **once**. Replacing the bytes behind a registered item would silently invalidate every signature and check referring to the old file; a correction is a new item with its own history.
- Frontend: `lib/api/custody.ts`, `hooks/use-custody.ts`, both screens on live data. Upload streams as multipart rather than base64 through the JSON client.

**Verified by deliberately tampering.** The test registers an item, attaches a file, confirms the digest matches `shasum` computed independently, transfers custody twice, then appends **one byte** to the stored file outside the platform. The next verification returns `broken` with both digests shown, both checks are kept, and the access log carries the failure.

**Not included, by design** — external anchoring. `blockchain_anchor_tx` exists and nothing writes it. The record is already tamper-evident; anchoring adds third-party corroboration and is a later layer.

**Open** — the custody signature is an HMAC proving the platform recorded it and that it has not been altered. It is not a personal digital signature bound to an officer's own key pair, which would need a PKI the deployment does not have.

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
| 2026-09-14 | Added `deploy/oracle-free/` — a 1 GB-shape profile for free demo hosting. Possible only because the Phase 02 storage abstraction lets evidence files sit on local disk instead of MinIO. |
| 2026-09-14 | **Phase 02 Evidence & Chain of Custody completed.** Storage abstraction (filesystem default, MinIO optional), streaming SHA-256 on upload, signed custody transfers, append-only access log, verification with full history, court verification view. Migrations `000030`/`000031`. Proven by tampering with a stored file and confirming the check catches it. |
| 2026-09-14 | Phase 01 re-verified before starting Phase 02 — both suites pass, zero audit failures. |
| 2026-09-13 | **Phase 01 completed in full.** Officer directory, evidence attachment with picker, relationship graph from real records, vehicle and location counts (migration `000028`). Found and fixed a defect blocking every insert into ten core tables: federated change-capture triggers with an unsatisfiable foreign key (migration `000029`), plus an FIR priority enum default. |
| 2026-09-13 | Created this plan of action. |
| 2026-09-13 | Edge hosting hardening: local Postgres, `bootstrap-db.sh`, `deploy/edge/`, `backup.sh`; five migration-chain bugs fixed; federation code deleted; IP lookup moved server-side; API contract and OpenAPI published and served at `/openapi.yaml`. |
| 2026-09-13 | Phase 01 Investigation Copilot completed end-to-end — migration `000026`, 8 tables, ~27 routes, React Query hooks, both screens on live data. Fixed three pre-existing platform bugs: every audit write was silently failing, the session middleware locked clients out after 3 requests, and logout never released the session. |
| 2026-09-13 | UI/UX reconstructed for Kolkata Police — design system, bilingual shell, all 14 module screens, no dead controls. |
| 2026-09-13 | Backend split into `Tatu1984/npdms-backend`; Go API repaired from 262 compile errors to a clean build. |
