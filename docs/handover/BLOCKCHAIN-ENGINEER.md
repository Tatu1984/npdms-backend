# Handover: anchoring the audit trail

For the engineer taking on blockchain anchoring. The plan is
`docs/plans/ai-and-anchoring-plan.html` (workstream B) and the anchoring
section of `POA.md`.

Read section 1 before anything else. The most common way this work goes wrong
is not technical.

---

## 1. What anchoring proves, and what it does not

The platform is already tamper-**evident**: every audit entry carries the hash
of its own fields plus the previous entry's hash, custody transfers are signed,
and evidence files are re-hashed on verification. Altering a record breaks the
chain and is detectable.

What that cannot do is prove **when** the chain existed. The police hold the
whole chain, so in principle they could rebuild it. Anchoring publishes a
fingerprint outside the platform so that a court or auditor can satisfy
themselves a given state existed by a given time without trusting the police at
all.

**An anchor proves that one 32-byte value existed no later than a stated time.
It does not prove:**

- that the records under it are true;
- that they were not altered *before* the batch was made — a record altered and
  then anchored is anchored in its altered form;
- who created them.

Say this plainly in every screen you build and in anything that goes to
counsel. The failure mode of this feature is not a bug; it is somebody in a
courtroom believing it proves more than it does.

**Only hashes leave the platform.** One Merkle root per batch. No record, no
personal data, no case content. The root commits to every entry in the batch
while revealing none of them, and cannot be reversed into them. This is a hard
rule from the plan.

---

## 2. What is already built

Migration `000081`, package `internal/anchor`. Roughly a third of the work.

- **`internal/anchor/merkle.go`** — RFC 6962 Merkle trees, the structure
  Certificate Transparency uses, chosen because auditors have seen it and
  because it is not vulnerable to the second-preimage confusion a naive tree
  has (leaves are hashed with a `0x00` prefix, interior nodes with `0x01`).
  An odd node is **carried up, not duplicated** — duplicating it, as Bitcoin
  does, lets two different sets of leaves produce the same root, which is
  exactly the property an anchor must not have. `Root`, `Proof` and
  `VerifyProof` are deliberately written to be reimplemented by someone who
  does not trust this codebase.
- **`internal/anchor/witness.go`** — two witnesses behind one interface,
  chosen because they fail differently:
  - **RFC 3161** (`TSAWitness`): a timestamping authority signs "this hash was
    presented to me at this time". Answers in about a second; the token
    verifies offline against the authority's certificate; but it is the
    authority's word.
  - **OpenTimestamps** (`OpenTimestampsWitness`): the hash is committed into
    Bitcoin through free public calendars. Nobody's word is required, but the
    block that settles it takes hours.
- **`anchor_receipts`** (migration 000081) — one row per witness per batch,
  holding the proof exactly as the witness returned it, append-only except to
  record a later confirmation.
- **`audit_log_batches`** already existed, reserved for this, with
  `merkle_root`, `start_sequence`, `end_sequence`, `record_count` and an
  `anchor_status`. `audit_logs` carries `sequence_number`, `previous_hash`,
  `current_hash` and `anchor_batch_id`. Build on these; do not invent new
  tables.

### Both free paths were tested end to end on 16 September 2026

Not read about — run:

- **OpenTimestamps.** A proof for a real digest in **3.6 seconds**, free, no
  account, submitted to four public calendars. The `.ots` file this Go code
  writes was verified with the reference `ots` client, which knows nothing
  about this platform: `ots verify -d <hex digest> root.bin.ots` reads it and
  reports three calendars pending confirmation in Bitcoin. Confirmation itself
  was still pending 4.5 hours later, which is normal.
- **FreeTSA (RFC 3161).** A token in **1.1 seconds**, free, no account, and
  `openssl ts -verify` against the authority's certificate returns
  **`Verification: OK`** — offline, by a third party, with no involvement from
  us.

Both are free of licence cost and per-call cost. The Go library for RFC 3161 is
`github.com/digitorus/timestamp` (BSD-2-Clause), already in `go.mod`.

---

## 3. What is left to build

In the order I would do it.

1. **The batching job.** Collect audit entries since the last batch by
   `sequence_number`, build leaves from each entry's `current_hash`, compute
   the root, write an `audit_log_batches` row, and stamp `anchor_batch_id` on
   the entries. Decide the cadence with Kolkata Police — hourly and on demand
   is a reasonable start. It must be idempotent and safe to run twice, and it
   must never renumber or rewrite an entry.
2. **Submit to both witnesses** and store the receipts. A witness that is not
   configured must be reported as not configured, not silently skipped — the
   platform's convention everywhere else is to say "not connected" rather than
   show an empty result. `ErrNotConfigured` already exists for this.
3. **The upgrade path.** An OpenTimestamps proof starts as a pending
   attestation and becomes a Bitcoin attestation hours later. Something must
   revisit pending receipts, upgrade them, and record `confirmed_at` and the
   block height. Until that exists the platform will show proofs that never
   finish, which looks worse than not having them.
4. **Verification, which is the actual product.** Given any audited record,
   produce: the record's own hash, its Merkle path, the batch root, and the
   witness proofs — and a plain-English page explaining how a third party
   checks each step with tools that are not ours (`ots verify`, `openssl ts
   -verify`). A proof nobody outside can check is theatre. Recompute the path
   from the audit rows rather than storing it, so nothing can fall out of sync.
5. **The screens.** A batch list with status, a record-level "prove this"
   action, and the verification page above. Bilingual, and honest about
   section 1 on the face of the screen.
6. **Air-gapped operation.** The edge server may have no route to a calendar.
   Queue batches and anchor when connectivity exists; `ANCHOR_OTS_CALENDARS=none`
   already means "there is no calendar here", and the screens must say so
   rather than appearing to anchor.

Then extend beyond the audit trail to the other things the plan names: Phase 02
custody, Phase 13 recordings, Phase 14 malkhana and case files, all of which
have `blockchain_anchor_tx` columns already reserved.

---

## 4. Decisions Kolkata Police must make, not you

- **Which anchoring target for production.** The free path is defensible for a
  demonstration and for a pilot. For production, the plan offers: legal
  timestamping only; a government ledger through NIC; a consortium with partner
  agencies; or a public chain. My recommendation is both a **CCA-licensed
  Indian timestamping authority** — so the token carries weight under the
  Information Technology Act rather than being a foreign free service — **and**
  OpenTimestamps, because they fail differently and together they are much
  harder to dispute. `ANCHOR_TSA_URL` is deliberately not hardcoded so the
  authority can be changed by configuration.
- **Retention and cadence**: how often batches are made, and how long proofs
  are kept.
- **Digital signature certificates** for officers (Class 3 DSC or eSign), so
  custody transfers and approvals carry a personal signature rather than only
  the platform's. This is listed in the plan as something the police must
  arrange and is independent of your work, but it is what turns "the platform
  says so" into "an officer signed it".
- A **security audit by a CERT-In empanelled auditor** before live use.

## 5. Things that will trip you up

- **Do not anchor on a schedule nobody watches.** A batch that fails silently
  is worse than no anchoring, because the screens will claim coverage that
  does not exist. Failures belong on a screen an officer sees.
- **Do not put anything but a hash on a public chain.** Bitcoin is permanent
  and public. A mistake here cannot be undone, and the plan's wording is
  explicit.
- **The `.ots` format is the point.** Write the standard file, not a private
  format. Check it with the reference client in your tests — `internal/anchor/witness_network_test.go`
  does exactly this behind `ANCHOR_NETWORK_TESTS=1`, and that test is how the
  current implementation was validated.
- Public testnets (Sepolia, Amoy) look attractive for a demo and are a trap:
  they are periodically reset, so a proof anchored there may not be verifiable
  later. OpenTimestamps on real Bitcoin is free and permanent; use it.
- The audit chain takes a transaction-scoped advisory lock when appending
  (see the changelog for 2026-09-14) because concurrent appends previously
  forked it. Your batching must not bypass that.

## 6. Where to start, concretely

```bash
cd services/api
ANCHOR_NETWORK_TESTS=1 ANCHOR_TSA_URL=https://freetsa.org/tsr \
  ANCHOR_TEST_OUTPUT=/tmp/anchor go test ./internal/anchor/ -v
```

That submits a real root to both witnesses and writes the artefacts. Then check
the file this codebase produced with the reference client, which is the habit
worth keeping:

```bash
pip install opentimestamps-client
ots info /tmp/anchor/root.bin.ots
ots verify -d $(xxd -p -c 64 /tmp/anchor/root.bin) /tmp/anchor/root.bin.ots
openssl ts -verify -data /tmp/anchor/root-tsa.bin -in /tmp/anchor/root.tsr \
  -CAfile cacert.pem -untrusted tsa.crt
```

`internal/anchor/merkle_test.go` is the shape of test to keep writing: it
proves every entry in batches of 1, 2, 3, 5, 7, 100 and 101 entries, proves an
altered entry no longer verifies, and proves two different batches never share
a root.
