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

# --- Pipeline tuning ---
TEXT_LAYER_MIN_CHARS = int(_get("TEXT_LAYER_MIN_CHARS", "100"))
RASTER_DPI = int(_get("RASTER_DPI", "200"))
RASTER_MAX_PX = int(_get("RASTER_MAX_PX", "1600"))
LLM_TEMPERATURE = float(_get("LLM_TEMPERATURE", "0"))

# --- Paths ---
PROJECT_ROOT = _PROJECT_ROOT
FIXTURES_DIR = _PROJECT_ROOT / _get("FIXTURES_DIR", "tests/fixtures")
LABELS_DIR = _PROJECT_ROOT / _get("LABELS_DIR", "tests/labels")

# Candidate models the benchmark harness scores against (the actual decision tool).
# Ordered small -> large so the scorecard reads "is the cheap one good enough?".
BENCH_MODELS = [MODEL_SMALL, MODEL_PRIMARY, MODEL_FALLBACK]
