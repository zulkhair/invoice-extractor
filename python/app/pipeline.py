"""Routing: text-layer path vs vision path, with fallback.

    PDF with text layer        -> text path  (cheap/fast)
    scanned PDF / image        -> vision path
    text path returns weak JSON-> fall back to vision path

"Weak" = fails Pydantic validation, or leaves a required-ish field null
(vendor_name, total_amount).
"""

from __future__ import annotations

import json
import re
from dataclasses import dataclass, field

from pydantic import ValidationError

from app import config, prompts
from app.ollama_client import ChatResult, OllamaClient, OllamaError
from app.pdf_text import extract_text_layer, is_pdf
from app.postprocess import normalize_raw
from app.rasterize import image_to_png, is_image, pdf_to_pngs
from app.schema import REQUIRED_ISH_FIELDS, Invoice


@dataclass
class ExtractionResult:
    invoice: Invoice
    path: str                     # "text" | "vision"
    model: str                    # resolved tag actually used
    latency_s: float
    fell_back: bool = False       # text path was tried then abandoned
    warnings: list[str] = field(default_factory=list)


class ExtractionError(RuntimeError):
    pass


def extract(
    data: bytes,
    filename: str = "",
    client: OllamaClient | None = None,
) -> ExtractionResult:
    """Run the hybrid pipeline over one document and return a validated Invoice."""
    client = client or OllamaClient()
    warnings: list[str] = []

    if is_pdf(data):
        layer = extract_text_layer(data)
        if layer.has_text_layer:
            text_res = _try_text(layer.text, client)
            if text_res is not None and _is_strong(text_res[0]):
                invoice, chat = text_res
                return _finish(invoice, "text", chat, warnings)
            warnings.append("text path weak/failed -> falling back to vision")
            images = pdf_to_pngs(data)
            return _vision(images, client, warnings, fell_back=True)
        # no text layer -> straight to vision
        images = pdf_to_pngs(data)
        return _vision(images, client, warnings)

    if is_image(data):
        return _vision([image_to_png(data)], client, warnings)

    raise ExtractionError(
        f"unsupported file type (not PDF or known image): {filename or '<upload>'}"
    )


# --- paths -----------------------------------------------------------------

def _try_text(text: str, client: OllamaClient) -> tuple[Invoice, ChatResult] | None:
    try:
        chat = client.chat(
            model=config.TEXT_PATH_MODEL,
            system=prompts.system_prompt(),
            user=prompts.text_user_prompt(text),
            json_format=True,
        )
    except OllamaError:
        return None
    invoice = _parse(chat.content)
    return (invoice, chat) if invoice is not None else None


def _vision(
    images: list[bytes],
    client: OllamaClient,
    warnings: list[str],
    fell_back: bool = False,
) -> ExtractionResult:
    # Two-model path: an OCR specialist transcribes the pixels, then the text-path
    # mapper turns that transcription into the schema (read-then-interpret).
    if config.OCR_MODEL:
        return _ocr_map(images, client, warnings, fell_back=fell_back)
    try:
        chat = client.chat(
            model=config.VISION_PATH_MODEL,
            system=prompts.system_prompt(),
            user=prompts.vision_user_prompt(),
            images=images,                # all pages in one call
            json_format=True,
        )
    except OllamaError as e:
        raise ExtractionError(f"vision path failed: {e}") from e
    invoice = _parse(chat.content)
    if invoice is None:
        raise ExtractionError("vision model did not return schema-valid JSON")
    return _finish(invoice, "vision", chat, warnings, fell_back=fell_back)


def _ocr_map(
    images: list[bytes],
    client: OllamaClient,
    warnings: list[str],
    fell_back: bool = False,
) -> ExtractionResult:
    """OCR the image(s) with the OCR specialist, then map the transcription to the
    schema with the text-path model. Latency is the sum of both calls; `model`
    reports both tags."""
    try:
        ocr = client.generate(
            model=config.OCR_MODEL,
            prompt=prompts.ocr_prompt(),
            images=images,
            num_predict=config.OCR_NUM_PREDICT,
        )
    except OllamaError as e:
        raise ExtractionError(f"OCR step failed: {e}") from e

    res = _try_text(ocr.content, client)
    if res is None:
        raise ExtractionError("OCR transcription did not map to schema-valid JSON")
    invoice, chat = res
    return ExtractionResult(
        invoice=invoice,
        path="vision",
        model=f"{config.OCR_MODEL} + {chat.model}",
        latency_s=round(ocr.latency_s + chat.latency_s, 3),
        fell_back=fell_back,
        warnings=warnings,
    )


def _finish(
    invoice: Invoice,
    path: str,
    chat: ChatResult,
    warnings: list[str],
    fell_back: bool = False,
) -> ExtractionResult:
    return ExtractionResult(
        invoice=invoice,
        path=path,
        model=chat.model,
        latency_s=round(chat.latency_s, 3),
        fell_back=fell_back,
        warnings=warnings,
    )


# --- helpers ---------------------------------------------------------------

def parse_model_content(content: str) -> Invoice | None:
    """Public: parse raw model output into a validated Invoice (or None).

    Used by the benchmark harness so it parses model output exactly as the live
    pipeline does."""
    return _parse(content)


def _parse(content: str) -> Invoice | None:
    """Parse model output -> normalized -> validated Invoice, or None if it
    can't be made schema-valid."""
    raw = _loads_lenient(content)
    if raw is None:
        return None
    try:
        return Invoice(**normalize_raw(raw))
    except (ValidationError, TypeError):
        return None


# Decode JSON numbers as their raw text, not Python int/float. A model that emits a
# locale-formatted amount as a bare number (e.g. 25.000 for "twenty-five thousand")
# would otherwise be decoded by json as 25.0 — the thousands-dot silently eaten.
# Keeping the original text lets postprocess.parse_decimal apply locale rules
# ("25.000" -> 25000), so it no longer matters whether the model quotes its numbers.
_RAW_NUMBERS = {"parse_int": str, "parse_float": str}


def _loads_lenient(content: str):
    """Tolerate stray prose/code-fences around the JSON object."""
    if not content:
        return None
    try:
        return json.loads(content, **_RAW_NUMBERS)
    except json.JSONDecodeError:
        pass
    m = re.search(r"\{.*\}", content, re.DOTALL)
    if m:
        try:
            return json.loads(m.group(0), **_RAW_NUMBERS)
        except json.JSONDecodeError:
            return None
    return None


def _is_strong(invoice: Invoice) -> bool:
    """Strong enough to skip the vision fallback: all required-ish fields present."""
    return all(getattr(invoice, f) not in (None, "") for f in REQUIRED_ISH_FIELDS)
