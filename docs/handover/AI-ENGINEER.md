# Handover: the AI layer

For the engineer taking on AI. Read this before writing code; most of it is
measured rather than assumed, and several of the measurements contradict what
the model cards say.

The plan this implements is `docs/plans/ai-and-anchoring-plan.html` (workstreams
A0–A5) and the AI section of `POA.md`. This document says what is already
built, what the rules are, what was measured, and what to do next.

---

## 1. The rules. These are not negotiable, and the database enforces them

They come from the plan agreed with Kolkata Police. Every one of them is
already held in SQL, not only in Go, because a rule that lives only in
application code is a rule until someone writes a second caller.

| Rule | Where it is enforced |
|---|---|
| A model's output is a lead, never a conclusion | `ai_decisions.status` has no AUTO_APPROVED value; a decided suggestion must name the officer who decided it (`trg_ai_decision_needs_an_officer`) |
| Measured before enabled | A model cannot be switched on until an evaluation of that exact version has passed (`trg_ai_model_enable_requires_evaluation`) |
| Confidence, sources, version on every suggestion | Columns on `ai_decisions`; provenance is immutable after the fact (`trg_ai_decision_provenance_is_fixed`) |
| Refuse without a source | The knowledge assistant answers only from a document it can cite — not yet built, see A1 |
| Places, never people | No model scores a person. Phase 10 scores areas and shows its factors |
| Data stays in jurisdiction | Every model runs on premises. There is no API key for an outside provider anywhere in this repository, and `ENV_VARIABLES.md` says why |
| Hashes on chain, never records | The anchoring engineer's problem — see `BLOCKCHAIN-ENGINEER.md` |
| Off by default, per module | `ai_module_switches`, one row per module, off at installation |

**If you find yourself wanting to relax one of these, that is a conversation
with Kolkata Police, not a code change.** The one that will tempt you is
auto-approval, because a queue of suggestions nobody reviews looks like
failure. It was in this codebase once — anything above 0.95 confidence was
marked AUTO_APPROVED, including for a model with no configuration at all — and
it was removed in commit b72176d. Do not put it back.

---

## 2. What is already built (workstream A0)

Migration `000076` and `000078`, package `internal/ai`, endpoints under
`/api/v1/ai-review`. Specified in `docs/api/openapi.yaml`.

- **The gateway** (`internal/ai/gateway.go`) is the only route from the
  platform to any model. It checks the registry and the module switch, calls
  the model, and records what came back as a PENDING suggestion. It never
  invents a result and never approves one.
- **The model registry** (`ai_model_configs`): name, version, module, task, the
  environment variable holding its service address, licence, source, threshold.
  Registering a model is `POST /ai-review/models` at SP rank and above. A model
  arrives switched off.
- **Evaluations** (`ai_model_evaluations`, append-only): dataset, size, metric,
  threshold, measured value, stated limitations, who ran it. A model is
  switched on only after one passes, and changing the version switches it off
  again. A measurement belongs to the registration it was run against, so a
  model removed and re-registered under the same name reads as unmeasured.
- **Monitoring**: acceptance and override rates per station, per language and
  per suggestion type, counted from the decisions themselves so the figures
  cannot disagree with them. `GET /ai-review/acceptance`.
- **The screens**: `/ai-oversight` (registry, evaluations, module switches,
  acceptance) and `/ai-review` (the officer's queue, showing each suggestion
  beside the record text it relied on).
- **The HTTP contract every model service must speak** (`internal/ai/http_client.go`):

      GET  {base}/health   -> {"service","modelName","modelVersion","licence","ready"}
      POST {base}/v1/infer -> {"prediction","confidence","data","alternatives","language","sources"}

  Address from an environment variable named in the registry; `_TOKEN` and
  `_TIMEOUT_SECONDS` variants are read automatically. **Where the variable is
  unset the model is "not connected" and every screen says so.** That is the
  state of the live deployment today and it is the correct state, not a defect.

**The registry ships empty.** Nothing on this platform calls a model until
someone registers one, measures it and switches it on. Your first working day
should end with you having done those three things against a stub service.

### Two precedents worth copying

Face recognition (`services/ml/face_recognition`, migrations 000072/000073) and
ANPR (`services/ml/vehicle_detection`, 000074/000075) are complete, working,
on-premises model services with authorisation, switches, purpose-logged
searches and immutable candidate evidence. They predate the gateway and call
their own clients directly. **Read them before you build anything** — they are
the house style for an AI feature, including how they behave when the service
is absent. Folding them onto the gateway is a reasonable early task.

---

## 3. What was measured, and what it means for you

### 3.1 OCR for Indian languages — built, and its limits are known

`internal/knowledge/ocr.go`, migration `000080`. Tesseract 5 with the
Apache-2.0 models for Bengali, Hindi, Marathi, Tamil, Telugu, Kannada,
Malayalam, Gujarati, Punjabi, Odia, Assamese, Urdu, Sanskrit, Nepali and
English. Free, CPU-only, no API key. The script is detected from the page, so
an officer never picks a language.

Measured on clean rendered text, character accuracy then whole words recovered:

| | characters | whole words |
|---|---|---|
| English | 100% | 100% |
| Marathi | 98% | 88% |
| Hindi | 93% | 67% |
| Telugu, Gujarati, Punjabi | 91–93% | 50–67% |
| Malayalam, Odia, Kannada | 78–86% | 0–50% |
| **Bengali** | **72–80%** | **12%** |
| Tamil | 74–76% | 0% |

Two findings that matter more than the table:

1. **The "best" models beat the "fast" ones enormously on Indic scripts** — on
   a Bengali page, 38% of words against 9%. The Docker image fetches the best
   models, pinned to a commit. Do not switch to `tessdata_fast` to save 150 MB.
2. **Pre-base vowel reordering.** Bengali, Devanagari, Tamil, Malayalam,
   Gujarati, Gurmukhi and Odia write some vowel signs to the left of the
   consonant they follow, so an engine reading glyphs left to right emits them
   one consonant late: পুলিশ comes back as পুলশি. Repairing that is arithmetic
   and lifted a Bengali page from 38.5% to 49.7% of characters. See
   `reorderPreBaseVowels`.

**What OCR is for here: finding a document, not reading it.** The text is
indexed with trigrams, so a mangled Bengali scan is still found by searching
কলকাতা, পুলিশ, অভিযোগ or থানা — all seven terms tried matched at 0.55 or
better against text where only 38% of words were letter-perfect. Every page
records the engine, languages, script, confidence and a note telling the
officer to use it to find the document, not to quote it. **Do not remove that
note, and do not let OCR text become record content.**

If you want better Indic OCR, that is a genuine research task. EasyOCR was
tried and is no better on Bengali, worse on Telugu, and its Tamil model fails
to load. Surya is strong but check its licence carefully before proposing it.

### 3.2 Local language models — the licensing minefield

Verified in September 2026 from primary sources. **Check again before you
commit to anything; three of these positions changed within twelve months.**

Safe (Apache-2.0, no additional terms, no attribution obligation):
- **Qwen3-4B-Instruct-2507** — the sensible interactive default. ~2.5 GB at
  Q4_K_M, ~4 GB RAM at 8k context.
- **Qwen3-8B** — better, but background jobs only on CPU.
- **sarvam-30b** (Sarvam AI) — Apache-2.0, Indian-built, Bengali native, 23
  Indian languages, MoE with only 2.4B active parameters. ~19.6 GB, needs a
  32 GB+ server. The best Bengali option that is actually licensed for
  government use. Needs llama.cpp from May 2026 or later (`sarvam_moe`).
- Phi-4 family (MIT), OLMo 2/3, IBM Granite, SmolLM — all clean, but **no
  Indic language coverage**, so English-only features.

**Do not use, all non-commercial or discretionary:** Qwen2.5-**3B** (research
licence — the most likely accidental adoption, because 3B is the size you
reach for), Ministral-8B-2410, EXAONE, **Sarvam-1**, **BharatGPT-3B-Indic**,
**Param-1/Param2**, **Krutrim-2**. The bitter irony is that most
India-funded, India-branded models are licensed so that an Indian government
department cannot use them without a negotiated deal. sarvam-30b is the
exception.

**Conditional, needs counsel:** Llama (must display "Built with Llama" in the
UI or product documentation if an internal system counts as "making
available", and any fine-tune must be named `Llama-*`); Gemma 2/3 (Google
reserves a contractual right to restrict usage remotely, and its prohibited
use policy bars "tracking or monitoring people without their consent" — the
closest any of these policies comes to hitting policing head-on); Gemma 4
(licence text is plain Apache-2.0 but the weights are access-gated behind
terms that could not be read anonymously — have counsel read the gate before
an engineer clicks it); `sarvam-translate` is **GPL-3.0**.

### 3.3 The two facts that should drive your architecture

1. **Small models do not understand Bengali usefully.** MILU (AI4Bharat,
   NAACL 2025) puts Llama-3.2-3B at 32.7% and Gemma-2-2B at 29.8% on Bengali
   multiple choice — barely above chance. No independent Bengali benchmark
   exists for Qwen3-4B or for sarvam-30b; Sarvam's own "wins 89% of
   comparisons" is vendor marketing with no baseline named. **Do not promise
   Bengali generation quality to Kolkata Police on the strength of a model
   card.** Budget a human evaluation on real Kolkata Police text first.
2. **Bengali tokenisation costs 3–8× more tokens than English** with a stock
   tokenizer (Sarvam reports 4–8 tokens per word for Indic against ~1.4 for
   English). A 2,000-token English prompt becomes 6,000–16,000 in Bengali.
   On a CPU-only server that turns a 6-second answer into 20–45 seconds.
   **Bengali generation on CPU is background-job work, not interactive**,
   unless you use an Indic-tokenizer model.

CPU performance, measured or from primary benchmarks: a 3–4B model at Q4_K_M
on an x86 server gives roughly 5–7 s to first token on a 2,000-token English
prompt; 7–8B gives 13–18 s for a single user and does not parallelise
gracefully. **The Apple development machine will lie to you** — llama.cpp uses
Metal there by default, which is 5–12× faster at prefill than the CPU-only
production server. Benchmark the actual procured SKU with `llama-bench`.
Buy one fat socket, not two: cross-NUMA access dropped generation from 8–10
tok/s to 2–5 tok/s in llama.cpp's own measurements.

Serving: **use llama.cpp's `llama-server`, not Ollama**, in production. Both are
MIT. Ollama's desktop application phones home hourly to `ollama.com/api/update`
with OS, architecture, version, timestamp and a macOS device identifier, with
no documented off switch. On a police project that is a conversation you want
to have before an audit, not after.

---

## 4. What to do next, in order

**A1 — Documents and language** is the plan's first workstream and the right
place to start: lowest risk, because every suggestion is checked against text
the officer is already reading.

1. **Stand up one model service** speaking the `/health` and `/v1/infer`
   contract above. Register it, record an evaluation, switch it on, watch a
   suggestion appear in `/ai-review`. Do this with something trivial first —
   the point is to exercise the whole path before the model matters.
2. **Suggested BNS sections when an FIR is registered.** The statute library is
   already loaded (`legal_acts`, `legal_sections`, migration 000068) with the
   full BNS, BNSS, BSA and IPC. Start with retrieval over that text plus the
   FIR's own words — embeddings and a citation, not generation. Every
   suggestion must carry the sentence from the FIR that prompted it, in
   `ai_decisions.sources`. This needs no LLM and cannot hallucinate a section
   that does not exist, which is the whole argument for doing it this way.
3. **Semantic search and cited answers in the knowledge assistant.** The
   documents, their classification and a trigram index already exist
   (`knowledge_documents`, migration 000058). Add embeddings alongside the
   existing `search_vector` — hybrid, not a replacement. **The assistant must
   refuse when no document covers the question.** Recommended embedding models,
   both permissive and multilingual: `BAAI/bge-m3` (MIT) or
   `Qwen/Qwen3-Embedding-0.6B` (Apache-2.0).
4. **Complaint category and routing for Phase 09**, and **entity extraction
   for Phase 05** (phones, UPI IDs, bank accounts from cyber-fraud complaint
   text). Note that `complaint_entities.recorded_by` is `NOT NULL REFERENCES
   users(id)`, so a machine-extracted entity cannot be inserted as-is — stage
   it in `ai_decisions` until an officer confirms it, which is the correct
   behaviour anyway.

Then A2 (investigation copilot), A3 (speech — note the old transcription
service was retired because it sent audio to Google), A4 (video — ANPR is
done, appearance matching and speed estimation are not), A5 (operations
analytics).

## 5. Things that will trip you up

- `origin` columns (`officer | derived | ai`) already exist on the relevant
  tables, reserved for you. Nothing writes `ai` yet. When you do, the record
  must also name the approving officer.
- `services/ml/` still holds `fir_classifier`, `semantic_search`, `ocr`,
  `ocr_service` and `nlp_extractor`. The plan marks them Rebuild/Merge, not
  Retire. They are prototypes: `ocr` and `ocr_service` both claim `ML_OCR_URL`
  and port 8004 with different APIs. Three others were retired outright
  (commit 396075b) — see `services/ml/README.md`.
- The live deployment is Vercel serverless. It cannot run a model service and
  has no tesseract binary, so OCR and every model report "not connected"
  there. The edge image carries them. Do not try to make Vercel run models.
- Run `scripts/probe-ai-gateway.py` after any change to the gateway. It is 45
  checks and it exercises the refusals, which is where the value is.

## 6. What you need from Kolkata Police before any of this goes live

Not your job to obtain, but your job to refuse to proceed without:

- Written authorisation per AI use case, and a DPIA under the DPDP Act 2023
  with a named data protection officer.
- An SOP issued to officers stating that AI output is a lead, before the first
  suggestion appears on a screen.
- Labelled examples from real work — around 1,000 FIRs with correct BNS
  sections, checked by an experienced investigating officer — because a model
  is measured against these before an officer ever sees a suggestion, and the
  evaluation gate will not let you switch anything on without them.
- GPU hardware at the MDC edge site, and a security audit by a CERT-In
  empanelled auditor.
