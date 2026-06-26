"""Central configuration, loaded from environment / .env.

Kept dependency-free (no python-dotenv) so the service starts with sane defaults
even before .env exists. Values mirror .env.example.
"""

from __future__ import annotations

import os
from pathlib import Path

_PROJECT_ROOT = Path(__file__).resolve().parent.parent


def _load_dotenv(path: Path) -> None:
    """Minimal .env loader: KEY=VALUE lines, no export, no interpolation.

    Real environment variables always win over .env (so CI / shell overrides work).
    """
    if not path.exists():
        return
    for raw in path.read_text().splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, _, value = line.partition("=")
        key = key.strip()
        # strip inline comments and surrounding quotes
        value = value.split("#", 1)[0].strip().strip('"').strip("'")
        os.environ.setdefault(key, value)


_load_dotenv(_PROJECT_ROOT / ".env")


def _get(key: str, default: str) -> str:
    return os.environ.get(key, default)


# --- Ollama ---
OLLAMA_HOST = _get("OLLAMA_HOST", "http://127.0.0.1:11434")
OLLAMA_TIMEOUT_S = float(_get("OLLAMA_TIMEOUT_S", "180"))
# Force N model layers onto the GPU (Ollama `num_gpu`). Ollama's auto-estimate can
# under-offload vision models — leaving VRAM idle while spilling layers to CPU. Set
# to the layer count or a large value (e.g. 99) to force full-GPU. Empty/None = let
# Ollama decide (the safe default for unknown GPUs).
_num_gpu_raw = _get("OLLAMA_NUM_GPU", "").strip()
OLLAMA_NUM_GPU = int(_num_gpu_raw) if _num_gpu_raw.lstrip("-").isdigit() else None
# Keep the model resident after a request: "30m", "-1" (forever), or empty (Ollama
# default ~5m). Staying warm avoids re-paying the one-time first-load vision warmup.
OLLAMA_KEEP_ALIVE = _get("OLLAMA_KEEP_ALIVE", "").strip()
# Context window (Ollama `num_ctx`). The default 4096 is too small for the vision
# path: a rasterized page is ~2k+ image tokens, plus the prompt, leaving no room for
# the JSON output -> it gets truncated mid-object. 8192 fits image + prompt + output.
OLLAMA_NUM_CTX = int(_get("OLLAMA_NUM_CTX", "8192"))

# --- Models (see README for verified tags / substitutions) ---
# Candidate pool (purpose-built + general VLMs), all Q4, all vision-capable:
MODEL_PRIMARY = _get("MODEL_PRIMARY", "frob/nuextract-2.0:8b-q4_K_M")   # NuExtract 2.0
MODEL_SMALL = _get("MODEL_SMALL", "qwen2.5vl:3b-q4_K_M")                # small VLM
MODEL_FALLBACK = _get("MODEL_FALLBACK", "qwen2.5vl:7b-q4_K_M")          # large VLM
# Text-only NuExtract 1.x — a BENCH candidate only; not used by the live pipeline
# (no vision projector, and it wants its own template prompt):
MODEL_TEXT = _get("MODEL_TEXT", "nuextract")

# Models the LIVE pipeline actually calls. Both must reliably return JSON from our
# generic prompt; the vision path model must be vision-capable.
TEXT_PATH_MODEL = _get("TEXT_PATH_MODEL", MODEL_SMALL)        # cheap, fast, reliable
VISION_PATH_MODEL = _get("VISION_PATH_MODEL", MODEL_FALLBACK)  # stronger VLM for pixels
# Optional OCR/vision specialist. When set, the vision path OCRs the image to text
# with this model and then maps that transcription to JSON with TEXT_PATH_MODEL — a
# fast "read then interpret" split (e.g. OCR_MODEL=glm-ocr + a general TEXT_PATH_MODEL).
# Empty = legacy single-VLM vision path (VISION_PATH_MODEL reads the pixels directly).
OCR_MODEL = _get("OCR_MODEL", "").strip()
# Cap the OCR transcription length (Ollama `num_predict`). A receipt is a few hundred
# tokens, but some images send GLM-OCR into a repetition loop (7k+ tokens, ~25s). The
# real content fits well within this; the mapper ignores any trailing junk. 0 = no cap.
OCR_NUM_PREDICT = int(_get("OCR_NUM_PREDICT", "2048"))

# --- Pipeline tuning ---
TEXT_LAYER_MIN_CHARS = int(_get("TEXT_LAYER_MIN_CHARS", "100"))
RASTER_DPI = int(_get("RASTER_DPI", "200"))
RASTER_MAX_PX = int(_get("RASTER_MAX_PX", "1600"))
LLM_TEMPERATURE = float(_get("LLM_TEMPERATURE", "0"))
# Default ISO-4217 currency when a receipt prints no currency code (many ID receipts
# show only "Rp" or bare numbers). Empty = never fabricate (leave null). Opt-in.
DEFAULT_CURRENCY = _get("DEFAULT_CURRENCY", "").strip() or None

# --- Paths ---
PROJECT_ROOT = _PROJECT_ROOT
FIXTURES_DIR = _PROJECT_ROOT / _get("FIXTURES_DIR", "tests/fixtures")
LABELS_DIR = _PROJECT_ROOT / _get("LABELS_DIR", "tests/labels")

# Candidate models the benchmark harness scores against (the actual decision tool).
# Ordered small -> large so the scorecard reads "is the cheap one good enough?".
BENCH_MODELS = [MODEL_SMALL, MODEL_PRIMARY, MODEL_FALLBACK]
