# Architecture

A local, self-hosted **receipt / invoice → schema-validated JSON** service. A
browser UI (or any HTTP client) uploads a document; a FastAPI backend routes it
through a hybrid text/vision pipeline, deterministically post-processes the model's
output, and returns a validated `Invoice` plus metadata. All inference runs locally
via **Ollama** — documents never leave the machine.

## Components & deployment

The backend runs in a container; **Ollama runs natively on the host** so it can use
the GPU (Docker can't reach the host GPU on Windows/macOS). The container reaches it
over HTTP via `host.docker.internal`.

```mermaid
flowchart LR
    user(["User / Browser"])

    subgraph container["Docker container — invoice-extractor"]
        direction TB
        ui["Web UI<br/>web/index.html"]
        api["FastAPI<br/>GET / · GET /health · POST /extract"]
        pipe["Pipeline + Postprocess<br/>PyMuPDF · Decimal · vendor rules · Pydantic"]
        ui --- api
        api --> pipe
    end

    subgraph host["Host — native, GPU"]
        direction TB
        ollama["Ollama<br/>:11434"]
        gpu["Local GPU<br/>quantized GGUF model(s)"]
        ollama --> gpu
    end

    user -->|"upload PDF / image"| ui
    pipe -->|"POST /api/chat<br/>host.docker.internal:11434"| ollama
    ollama -->|"raw JSON"| pipe
    pipe -->|"invoice + metadata (JSON)"| user
```

## Request flow (`POST /extract`)

The pipeline is **hybrid**: a digital PDF with a text layer takes the cheap text
path; scans and images take the vision path; a weak text result falls back to
vision. Whatever the path, the model's output is treated as untrusted — numbers and
dates are re-parsed deterministically and totals are re-checked.

```mermaid
flowchart TD
    A["POST /extract<br/>(PDF or image bytes)"] --> B{"PDF?"}
    B -->|"image"| RAST["Rasterize page → PNG<br/>(PyMuPDF)"]
    B -->|"PDF"| C{"Has text layer?<br/>(PyMuPDF heuristic)"}
    C -->|"yes"| TEXT["Text path<br/>qwen2.5vl:3b on extracted text"]
    C -->|"no"| RAST
    TEXT --> STRONG{"Strong result?<br/>vendor + total present"}
    STRONG -->|"no → fall back"| RAST
    RAST --> VIS["Vision path<br/>VLM on the image,<br/>or optional OCR→map two-model"]
    STRONG -->|"yes"| NORM
    VIS --> NORM

    NORM["Parse + normalize (deterministic)<br/>• lenient JSON (numbers kept as raw text)<br/>• locale → Decimal, datetime<br/>• category via vendor-keyword rules<br/>• total = sum of line item prices"]
    NORM --> VAL["Validate<br/>(Pydantic Invoice schema)"]
    VAL --> OUT["200 OK<br/>invoice + metadata<br/>(path, model, latency, warnings)"]

    TEXT -. "Ollama /api/chat" .-> OLL[("Ollama / GPU")]
    VIS  -. "Ollama /api/chat" .-> OLL
```

## Key design principles

- **Hybrid routing** — cheap text path for digital PDFs; vision path for scans/photos;
  automatic fallback when the text result is weak. (`app/pipeline.py`)
- **Optional OCR specialist** — set `OCR_MODEL` and the vision path becomes two models: a
  dedicated OCR model (e.g. `glm-ocr`) transcribes the image to text via `/api/generate`,
  then `TEXT_PATH_MODEL` maps that text to the schema. The OCR model reads messy receipts
  well while a general model handles the semantics; left unset, the single-VLM vision path
  is used. (`app/pipeline.py`)
- **Never trust the model** — the prompt asks it to copy values verbatim; all number
  formatting (locale `1.250.000,00` → `Decimal`) and date/time parsing happen in
  deterministic Python, and the **total is computed by summing the line item prices**
  (the model's printed grand total — often the cash tendered — is ignored).
  (`app/postprocess.py`)
- **Schema is the single source of truth** — the JSON schema injected into the prompt
  is generated from the Pydantic `Invoice` model, so prompt and validator can't drift.
  Every response is validated; absent fields are `null`, never fabricated. (`app/schema.py`)
- **Category is a rule, not the LLM** — spending category is assigned by a
  deterministic vendor-keyword map (overriding the model), which is far more reliable
  for known chains than asking a small model. (`app/postprocess.py`)
- **Local & GPU-native** — inference is Ollama on the host GPU; the container is just
  the app. Quantized GGUF models only (Q4), sized to fit modest VRAM.
