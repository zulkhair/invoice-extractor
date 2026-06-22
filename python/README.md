# Invoice Extractor (local, Ollama)

Self-hosted invoice → schema-validated JSON. PDF or image in, clean `Invoice`
JSON out. Inference runs **locally via Ollama** — documents never leave the
machine. Quantized GGUF models keep VRAM low, so it runs on modest GPUs (and CPU).

## Hard constraints (do not violate)

- **Ollama only.** No vLLM — runs on older GPUs (e.g. Pascal, compute 6.1) where vLLM (needs 7.0+) won't.
- **Quantized GGUF only — never FP16.** Older GPUs run FP16 far slower than FP32. Use Q4/Q5.
- **Fit your VRAM.** Keep model + projector + context within available VRAM (defaults fit ~6 GB).
- **Verify model tags before hardcoding** — registry tags drift (`ollama show <tag>`).
- **Vision must actually work** — every vision model must pass the vision sanity check.

## Architecture (hybrid)

```
Invoice (PDF/image)
  ├─ has embedded text layer? ─(PyMuPDF)─► text + schema ─► LLM (text mode) ─► JSON
  └─ scanned / no text layer  ─(rasterize)─► image + schema ─► VLM (vision) ─► JSON
                                                                   │ validate (Pydantic)
                                                   low-confidence / null required fields?
                                                                   └─► retry with vision model
```

Text-mode calls are cheaper/faster and keep the single GPU free; only scans/images
hit the heavy vision model.

## Model tags (VERIFIED 2026-06-21)

**"NuExtract3 4B" does not exist.** Verified substitutions:

| Role | Resolved tag | Size | Vision | Notes |
|------|--------------|------|--------|-------|
| Primary (purpose-built extract) | `frob/nuextract-2.0:8b-q4_K_M` | 6.0 GB | ✓ | NuExtract 2.0 (built on Qwen2.5-VL). Only 8B on Ollama — no 4B. Community upload. |
| Small vision | `qwen2.5vl:3b-q4_K_M` | 3.2 GB | ✓ | Official library. Smallest/cheapest candidate. |
| Fallback (general VLM) | `qwen2.5vl:7b-q4_K_M` | 6.0 GB | ✓ | Official library. |
| Fast text path | `nuextract` | ~2.2 GB | ✗ | NuExtract 1.x (Phi-3 3.8B). **Text-only — fails the vision check by design.** Use only for digital PDFs. |

`scripts/pull_models.sh` re-verifies each tag against the live registry before pulling
and prints what it resolved — treat the table above as the starting point, not gospel.

## Setup (fresh Linux / WSL2 box)

System packages need `sudo` (run yourself):

```bash
bash scripts/setup_env.sh        # installs Ollama + poppler-utils, prints next steps
```

Python environment (no sudo):

```bash
python3 -m venv .venv
.venv/bin/pip install -e ".[dev]"
cp .env.example .env
```

Pull + verify models (Ollama must be running — `ollama serve`):

```bash
bash scripts/pull_models.sh
```

## Run

```bash
.venv/bin/uvicorn app.main:app --reload      # http://127.0.0.1:8000
curl -s localhost:8000/health                # {"status":"ok",...}
curl -s -F file=@invoice.pdf localhost:8000/extract | python -m json.tool
```

`/extract` returns the validated `Invoice` (vendor, transaction datetime, currency,
category, line items, and a `total_amount` computed by summing the line item prices)
plus metadata: `path` taken (text|vision), `model` + resolved tag, `latency_s`, `warnings`.

A simple **web UI** is served at the root — open <http://127.0.0.1:8000> to drag in a
receipt and see the rendered result plus raw JSON.

## Run with Docker

The backend + UI run in a container; **Ollama stays on the host** (so it keeps the
GPU — Docker can't reach the host GPU on Windows/macOS). Pull the models first, then
from the repo root:

```bash
docker compose up --build        # then open http://localhost:8000
```

The container reaches the host's Ollama via `host.docker.internal`. Override models,
timeouts, or `OLLAMA_HOST` in `docker-compose.yml`. (If you run Ollama in a container
too — Linux/CPU only — point `OLLAMA_HOST` at that service instead.)

## Test & benchmark

```bash
.venv/bin/pytest                             # unit tests (postprocess, schema, scoring)
.venv/bin/python scripts/bench.py            # per-model, per-field accuracy scorecard
```

Drop real labeled invoices in `tests/fixtures/` (PDF/image) with matching
ground-truth JSON in `tests/labels/` (same stem, `.json`). Both dirs are
gitignored — real invoices never get committed. `bench.py` is the **decision tool**:
it shows which model gives the best accuracy/latency for your hardware.

## Acceptance checklist

| Capability | Gate |
|------------|------|
| Scaffold | `uvicorn app.main:app` starts, `/health` → 200 ✓ |
| Client + pull | both models pulled, `ollama list` shows them, trivial prompt returns |
| Vision check | ≥1 model demonstrably reads pixels (vision check script) |
| Text path | clean digital PDF → schema-valid JSON, no image sent |
| Vision path | scanned/image invoice → schema-valid JSON via vision |
| Postprocess | money fields are clean `Decimal`; consistency flag set |
| /extract | end-to-end curl returns documented response shape |
| Benchmark | `scripts/bench.py` prints per-model, per-field scorecard |
