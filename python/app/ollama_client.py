"""Thin HTTP client for Ollama's /api/chat (plus list/show for verification).

Kept deliberately plain (one POST, JSON in/out) so the interface ports cleanly
to Go later. No streaming — extraction wants a single deterministic response.
"""

from __future__ import annotations

import base64
import time
from dataclasses import dataclass, field

import httpx

from app import config


@dataclass
class ChatResult:
    content: str               # assistant message text (expected: a JSON object string)
    model: str                 # tag actually used
    latency_s: float
    raw: dict = field(default_factory=dict)
    eval_count: int | None = None       # tokens generated (perf signal for bench)


class OllamaError(RuntimeError):
    pass


class OllamaClient:
    def __init__(self, host: str | None = None, timeout_s: float | None = None):
        self.host = (host or config.OLLAMA_HOST).rstrip("/")
        self.timeout_s = timeout_s if timeout_s is not None else config.OLLAMA_TIMEOUT_S

    # --- introspection (used by pull verification + health) ---
    def ping(self) -> bool:
        try:
            r = httpx.get(f"{self.host}/api/tags", timeout=5)
            return r.status_code == 200
        except httpx.HTTPError:
            return False

    def list_models(self) -> list[str]:
        r = httpx.get(f"{self.host}/api/tags", timeout=10)
        r.raise_for_status()
        return [m["name"] for m in r.json().get("models", [])]

    def show(self, model: str) -> dict:
        """Model metadata; raises OllamaError if the tag is not present locally."""
        r = httpx.post(f"{self.host}/api/show", json={"model": model}, timeout=15)
        if r.status_code == 404:
            raise OllamaError(f"model not found locally: {model}")
        r.raise_for_status()
        return r.json()

    def supports_vision(self, model: str) -> bool:
        """Best-effort capability probe from /api/show families."""
        try:
            info = self.show(model)
        except (OllamaError, httpx.HTTPError):
            return False
        caps = info.get("capabilities") or []
        if "vision" in caps:
            return True
        families = (info.get("details") or {}).get("families") or []
        return any(f in {"clip", "mllama", "qwen2vl", "qwen2.5vl"} for f in families)

    # --- the one call that matters ---
    def chat(
        self,
        model: str,
        system: str,
        user: str,
        images: list[bytes | str] | None = None,
        json_format: bool | dict = True,
        temperature: float | None = None,
    ) -> ChatResult:
        """Single-shot chat. `images` are raw bytes or base64 strings (PNG).

        json_format: True -> Ollama `format: "json"`; a dict -> structured JSON
        schema; False -> free text.
        """
        user_msg: dict = {"role": "user", "content": user}
        if images:
            user_msg["images"] = [_to_b64(img) for img in images]

        payload: dict = {
            "model": model,
            "messages": [
                {"role": "system", "content": system},
                user_msg,
            ],
            "stream": False,
            "options": {
                "temperature": config.LLM_TEMPERATURE if temperature is None else temperature
            },
        }
        if json_format is True:
            payload["format"] = "json"
        elif isinstance(json_format, dict):
            payload["format"] = json_format

        t0 = time.perf_counter()
        try:
            r = httpx.post(
                f"{self.host}/api/chat", json=payload, timeout=self.timeout_s
            )
        except httpx.HTTPError as e:
            raise OllamaError(f"Ollama request failed ({self.host}): {e}") from e
        latency = time.perf_counter() - t0

        if r.status_code != 200:
            raise OllamaError(f"Ollama {r.status_code}: {r.text[:400]}")
        data = r.json()
        content = (data.get("message") or {}).get("content", "")
        return ChatResult(
            content=content,
            model=model,
            latency_s=latency,
            raw=data,
            eval_count=data.get("eval_count"),
        )


def _to_b64(img: bytes | str) -> str:
    if isinstance(img, str):
        return img  # already base64
    return base64.b64encode(img).decode("ascii")
