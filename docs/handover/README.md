# Handover

Two pieces of this platform are being taken on by other engineers. Each has its
own document:

- **[AI-ENGINEER.md](AI-ENGINEER.md)** — the AI layer. Workstream A0 is built
  (gateway, model registry, evaluation gate, module switches, acceptance
  monitoring) and OCR for fifteen Indian languages is live. A1 to A5 remain.
- **[BLOCKCHAIN-ENGINEER.md](BLOCKCHAIN-ENGINEER.md)** — anchoring the audit
  trail outside the platform. The Merkle layer and two free witnesses are
  built and tested against reference tooling; batching, upgrade, verification
  and screens remain.

The plan both implement is `docs/plans/ai-and-anchoring-plan.html`. The state
of the whole platform is `POA.md`, which is the system of record for what is
built and what is not — keep it current as you go.

---

## Conventions that hold across the whole platform

These are not house preferences. They are why this codebase can be shown to a
police force, and every one of them has been broken at least once and fixed.

**Nothing claims to work that does not.** A model service that is absent, a
camera that is not connected, an OCR engine that is not installed: the screen
says so, in plain words, and the API answers with a stated reason. It never
returns an empty list that reads as "there is nothing here". If you cannot do
something yet, say that on the screen.

**Rules live in the database as well as in Go.** A rule enforced only in
application code is a rule until somebody writes a second caller. Look at how
the evaluation gate, the force boundary and the referral rules are written as
triggers and constraints, and follow that. The Go layer then turns the
database's refusal into a sentence an officer can act on.

**A refusal names the rule.** Not "server error", not a 500 — the status that
fits (400, 403, 409) and a message saying which rule stopped it and what to do
instead. See `refusalMessage` in `internal/handlers/ai_review_handler.go` and
the `other_force` refusal in `internal/handlers/force_refusal.go`.

**Measured, not claimed.** If you state an accuracy figure, it must come from a
run you can point at, with its conditions and its limits written next to it.
The OCR figures in the AI handover are in that form deliberately. Where no
reliable figure exists, say that instead of estimating.

**Verify by driving it.** A phase is not done here until its forms have been
driven in a browser and the writes confirmed in Postgres. There are probe
scripts in `scripts/` — `probe-ai-gateway.py`, `probe-referrals.py`,
`probe-ocr.py`, `probe-endpoints.py` — and they exercise refusals, not just
happy paths. Add one for what you build; it is the fastest way to find the
schema-mismatch bugs this codebase is prone to.

**No fabricated data, ever.** No sample officers, no illustrative records, no
placeholder statistics presented as real. Demo data is created through the API
by `scripts/seed-*.py` so that numbering, workflow rules and audit entries are
genuine, and every fictional name is labelled as fictional.

**British English, plain words.** In code comments, commit messages, and
anything an officer reads. Explain why, not what.

---

## Getting the platform running locally

```bash
# Postgres, schema and reference data (idempotent)
./scripts/bootstrap-db.sh

# Demo stations, officers and records for all four departments
DEMO_SEED_CONFIRM=yes DATABASE_URL="postgresql://<user>@localhost:5432/npdms" \
  python3 scripts/seed-departments-demo.py

cd services/api && cp .env.example .env   # then edit
go run .

cd ../../../npdms/ui/web && npm install
NEXT_PUBLIC_API_URL=http://localhost:8080/api/v1 npm run dev
```

Demo logins all use the password `Demo@123`. Kolkata Police: `admin`, `sp`,
`dsp`, `sho`, `inspector`, `si`, `asi`, `hc`, `constable`. West Bengal Police:
`sp.n24`, `dsp.hwc`, `oc.brs`, `si.ttg`. CID: `sp.cid`, `dsp.cid`,
`insp.cid.cyb`, `si.cid.aht`. Traffic: `dc.traffic`, `oc.tg.pks`, `sgt.tg.jdp`,
`sgt.tg.hwb`, `sgt.pks`. Sign-in is rate limited to five a minute per address,
so a script should sign in once and reuse the session.

**These accounts share one password and are fictional. Rotate or remove them
before the platform holds a real record.**

## The tests

```bash
cd services/api
go build ./... && go vet ./...
TEST_DATABASE_URL="postgres://<user>@localhost:5432/npdms?sslmode=disable" go test ./...
```

Database-backed tests skip when `TEST_DATABASE_URL` is unset, so the suite is
still useful on a machine without Postgres. Coverage is thin outside the
packages that were built recently — treat a package you touch as one you leave
better covered than you found it.
