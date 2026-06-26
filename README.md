# Invoice Extractor (local, Ollama) — monorepo

Self-hosted invoice → schema-validated JSON. PDF or image in, clean `Invoice` JSON
out. Inference runs **locally via Ollama** — documents never leave the machine.
Uses quantized GGUF models, so it runs on modest consumer GPUs (and CPU) — anywhere
Ollama runs.

Two parallel implementations behind one `/extract` contract, so their benchmarks
are directly comparable:

| | Stack | Dir |
|--|-------|-----|
| **Python** | FastAPI · Pydantic · PyMuPDF · pdf2image | [python/](python/) |
| **Go** | net/http · shopspring/decimal · poppler · ollama/api | [go/](go/) |

```
python/   the Python app                 → python/README.md
go/        the Go app                    → go/README.md
scripts/   setup_env.sh, pull_models.sh  (SHARED: Ollama + poppler, model pull)
```

## Constraints
- **Ollama only**, **quantized GGUF only** (never FP16) — keeps VRAM low and runs on
  older GPUs where vLLM (needs compute 7.0+) won't.
- Size models to your VRAM (the defaults fit ~6 GB). Verify tags before pulling
  (`scripts/pull_models.sh`); every vision model must pass the vision check.
- Never fabricate: absent field → null/nil. Money uses `Decimal`, never float.

## Quick start (Linux / WSL2)
```bash
bash scripts/setup_env.sh        # sudo: installs Ollama + poppler, starts the server
bash scripts/pull_models.sh      # verify + pull models (refuses FP16)
python/.venv/bin/python python/scripts/make_synthetic_fixture.py   # fixtures for both apps
```
Then pick an app:
```bash
# Python
cd python && python3 -m venv .venv && .venv/bin/pip install -e ".[dev]" && cp .env.example .env
.venv/bin/pytest && .venv/bin/uvicorn app.main:app --reload

# Go
cd go && cp .env.example .env && go test ./... && go run ./cmd/server
```
Both expose `GET /health` and `POST /extract` (multipart file upload) and return the
validated invoice plus metadata (path taken, model + tag, latency, warnings).

## Example
`POST /extract` with an invoice file returns:
```json
{
  "invoice": {
    "vendor_name": "Indomaret Tlogomas 44 Malang",
    "transaction_datetime": "2022-01-21T18:27:00",
    "currency": "IDR",
    "category": "groceries",
    "line_items": [
      {"description": "POP MIE AYAM 75G", "amount": "4900"},
      {"description": "LE MINERALE 600ML", "amount": "7000"}
    ],
    "total_amount": "11900"
  },
  "metadata": {"path": "vision", "model": "qwen2.5vl:7b-q4_K_M", "latency_s": 8.2, "fell_back": false, "warnings": []}
}
```
It's an expense record: `category` is one of a fixed set of buckets, and `total_amount`
is **computed by summing the line item prices** (not read from the document). Absent
fields come back `null` (never fabricated). A runnable sample lives at
`python/tests/fixtures/synthetic_invoice.pdf` (+ ground truth in `tests/labels/`).

## Models (verified 2026-06-21)
An earlier candidate, **"NuExtract3 4B"**, **does not exist**. Resolved substitutions
(all Q4, sized for modest VRAM) are documented in each app's README. Live pipeline uses
`qwen2.5vl:3b-q4_K_M` (text) and `qwen2.5vl:7b-q4_K_M` (vision);
`frob/nuextract-2.0:8b-q4_K_M` is the purpose-built bench candidate.

## Benchmark
`bench.py` (Python) / `cmd/bench` (Go) produce per-model, per-field accuracy
scorecards on real invoices — use it to pick the best model for your hardware. Drop
real labeled invoices in `<app>/.../fixtures` + `labels` (gitignored) to run it.
