"""FastAPI service: /health and /extract."""

from __future__ import annotations

from pathlib import Path

from fastapi import FastAPI, File, HTTPException, UploadFile
from fastapi.responses import FileResponse, JSONResponse

from app import __version__, config
from app.ollama_client import OllamaClient
from app.pipeline import ExtractionError, extract

app = FastAPI(title="invoice-extractor", version=__version__)

_INDEX_HTML = Path(__file__).resolve().parent.parent / "web" / "index.html"


@app.get("/", include_in_schema=False)
def index() -> FileResponse:
    """Serve the single-page upload UI (same origin as /extract, so no CORS)."""
    return FileResponse(_INDEX_HTML)


@app.get("/health")
def health() -> dict:
    """Liveness + a best-effort note on whether Ollama is reachable.

    Returns 200 even if Ollama is down (the API process itself is healthy) —
    the `ollama` field tells callers whether extraction will actually work.
    """
    ollama_up = OllamaClient().ping()
    return {
        "status": "ok",
        "version": __version__,
        "ollama": "up" if ollama_up else "down",
        "ollama_host": config.OLLAMA_HOST,
        "text_path_model": config.TEXT_PATH_MODEL,
        "vision_path_model": config.VISION_PATH_MODEL,
    }


@app.post("/extract")
async def extract_endpoint(file: UploadFile = File(...)) -> JSONResponse:
    """Accept a PDF or image, run the hybrid pipeline, return validated Invoice
    JSON plus metadata (path taken, model + tag, latency, warnings)."""
    data = await file.read()
    if not data:
        raise HTTPException(status_code=400, detail="empty upload")

    try:
        result = extract(data, filename=file.filename or "")
    except ExtractionError as e:
        raise HTTPException(status_code=422, detail=str(e)) from e

    return JSONResponse(
        {
            "invoice": result.invoice.model_dump(mode="json"),
            "metadata": {
                "path": result.path,
                "model": result.model,
                "latency_s": result.latency_s,
                "fell_back": result.fell_back,
                "warnings": result.warnings,
                "filename": file.filename,
            },
        }
    )
