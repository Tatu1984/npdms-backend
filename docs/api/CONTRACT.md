# API contract

The Go API is the system of record. Every client — the web application, any
mobile client, and any integration from CCTNS, ICJS or an MDC system — reaches
the platform only through this contract. No business rule, no derived figure
and no authorisation decision belongs anywhere else.

This document is the contract's conventions. The machine-readable definition is
`openapi.yaml`, also served by the running server at `GET /openapi.yaml`.

## Base URL and versioning

```
http://<edge-server>:8080/api/v1
```

The version is in the path. Within `v1` the following are **additive only**:

- New endpoints.
- New optional request fields.
- New response fields.

A client must therefore ignore response fields it does not recognise, and must
not depend on field ordering. These are **breaking** and require `v2`:

- Removing or renaming a field.
- Narrowing a type, or making an optional request field required.
- Changing the meaning of an existing value.
- Changing an endpoint's status codes for the same condition.

Deprecated endpoints respond with a `Deprecation` header carrying the date they
stop being served, and remain for at least one further release.

## Authentication

```
POST /api/v1/auth/login     { "username": "...", "password": "..." }
  → { "accessToken": "...", "refreshToken": "...", "expiresIn": 3600, "user": {...} }
```

Send the access token on every subsequent request:

```
Authorization: Bearer <accessToken>
```

Tokens expire after an hour; exchange the refresh token at
`POST /api/v1/auth/refresh`. `POST /api/v1/auth/logout` blacklists the token and
releases the session.

**Sessions.** A sign-in is one session, identified by the access token. An
officer may hold `MAX_CONCURRENT_SESSIONS` (default 5) at once; beyond that,
requests are refused with `429`. Call `logout` when a client is finished — an
abandoned session occupies a slot until it expires.

Public endpoints, which need no token: `/auth/login`, `/auth/refresh`,
`/auth/logout`, `/public/*`, `/health`, `/ready`.

## Errors

Every error is the same shape:

```json
{ "error": "validation_error", "message": "Name is required", "code": 400 }
```

`error` is a stable machine-readable slug — branch on this, never on `message`.
`message` is for a person and may be reworded at any time.

| Status | Meaning |
|--------|---------|
| `400` | The request is malformed or fails validation |
| `401` | No token, or the token is invalid or expired |
| `403` | Authenticated, but the role is not permitted |
| `404` | No such record, or not visible to this officer |
| `409` | Conflicts with existing state, e.g. a duplicate case number |
| `429` | Rate limited, or the concurrent-session cap is reached |
| `500` | Server fault — the response body carries no internal detail |

## Pagination

List endpoints take `page` (from 1) and `pageSize` (default 20), and return:

```json
{ "data": [...], "total": 184, "page": 1, "pageSize": 20, "totalPages": 10 }
```

Sub-resource lists that are naturally bounded — the persons on one case, the
entries in one chronology — return `{ "data": [...] }` without pagination.

## Conventions

**Identifiers** are UUIDs, as strings.

**Timestamps** are RFC 3339 in UTC (`2024-11-18T20:47:00Z`). Dates without a
time are `YYYY-MM-DD`.

**Bilingual text** arrives as a pair: `title` carries English, `titleBn` carries
Bengali and may be absent. Clients choose per the user's language and fall back
to English.

**Derived values are computed by the server.** Counts, progress and status
rollups are calculated on read and returned in the response. A client must not
recompute them from sub-resources; if a figure is wanted that the API does not
return, the API should return it.

**Provenance.** Any record the platform derived rather than an officer entering
carries `origin`:

| `origin` | Meaning |
|----------|---------|
| `officer` | A person entered it |
| `supervisor` | A supervising officer entered it |
| `derived` | A deterministic rule produced it — the rule is named in `ruleKey` |
| `ai` | A model suggested it |

Nothing writes `ai` yet. When it does, the field already exists, and anything
carrying it must be presented to the user as advisory and reviewed by an
officer before it is acted on.

## Auditing

Every state change is appended to `audit_logs`, a hash-chained append-only
table: each row carries the SHA-512 of its own fields plus the previous row's
hash, so a removed or altered row breaks the chain and is detectable.

This is tamper evidence, not blockchain. Anchoring batches of these hashes to a
chain is a later phase; `blockchain_anchor_tx` is the column reserved for it.

Audit failures are logged by the server and never silently discarded.

## Rate limiting

100 requests per minute per client by default, returned in the usual headers:

```
X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset
```

Auth endpoints are limited more tightly.

## Health

| Endpoint | Purpose |
|----------|---------|
| `GET /health` | The process is up. Use for liveness. |
| `GET /ready` | Database and Redis are reachable. Use for readiness and for load-balancer checks. |

Neither requires a token.

## Surface

585 routes are registered, as of 16 September 2026. Those with a complete
specification in `openapi.yaml` are the ones whose behaviour is settled:

- `/auth/*` — sign-in, refresh, sign-out
- `/investigation/*` — Phase 01, the investigation workspace
- `/intel/ip/{ip}` — server-side address resolution
- `/health`, `/ready`

The remainder are implemented and callable but their request and response
shapes are still moving as each phase is made functional. Treat anything not in
`openapi.yaml` as unstable, and expect it to be specified as its phase lands.
`docs/api/routes.txt` lists every registered route so nothing is hidden. It is
taken from the routes the API itself registers at start-up, not from reading
the source, so it cannot quietly fall behind:

    ./api 2>&1 | grep 'GIN-debug.*-->' \
      | sed -E 's/^\[GIN-debug\] ([A-Z]+) +([^ ]+) +-->.*/\1 \2/' \
      | sort -u > docs/api/routes.txt
