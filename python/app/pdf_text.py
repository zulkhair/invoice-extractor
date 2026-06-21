"""PDF text-layer detection + extraction (PyMuPDF).

A digital invoice usually carries an embedded text layer; a scan does not. We use
a simple per-page char-count heuristic to decide whether the fast text path is
viable, and fall back to the vision path otherwise.
"""

from __future__ import annotations

from dataclasses import dataclass

import fitz  # PyMuPDF

from app import config


@dataclass
class TextLayer:
    text: str
    has_text_layer: bool
    page_count: int
    chars_per_page: list[int]


def extract_text_layer(
    pdf_bytes: bytes, min_chars_per_page: int | None = None
) -> TextLayer:
    """Extract embedded text and decide whether the layer is usable.

    Heuristic: a PDF "has a text layer" if its busiest page has at least
    `min_chars_per_page` extractable characters. One good page is enough — many
    invoices are a single page, and mixed PDFs (scanned cover + digital detail)
    still benefit from the text we can get.
    """
    threshold = (
        min_chars_per_page if min_chars_per_page is not None else config.TEXT_LAYER_MIN_CHARS
    )
    parts: list[str] = []
    counts: list[int] = []
    with fitz.open(stream=pdf_bytes, filetype="pdf") as doc:
        page_count = doc.page_count
        for page in doc:
            page_text = page.get_text("text") or ""
            parts.append(page_text)
            counts.append(len(page_text.strip()))

    has_layer = bool(counts) and max(counts) >= threshold
    return TextLayer(
        text="\n\n".join(parts).strip(),
        has_text_layer=has_layer,
        page_count=page_count,
        chars_per_page=counts,
    )


def is_pdf(data: bytes) -> bool:
    return data[:5] == b"%PDF-"
