"""Rasterize PDFs / normalize images to PNG for the vision path.

PDF pages are rendered with PyMuPDF (no poppler dependency for the render itself);
pdf2image/poppler is kept as an optional fallback. Images are loaded with Pillow,
converted to RGB, downscaled so the longest edge fits RASTER_MAX_PX (keep VRAM and
context in check), and emitted as PNG bytes.
"""

from __future__ import annotations

import io

import fitz  # PyMuPDF
from PIL import Image

from app import config


def pdf_to_pngs(
    pdf_bytes: bytes, dpi: int | None = None, max_px: int | None = None
) -> list[bytes]:
    """Render each PDF page to a PNG (one per page)."""
    dpi = dpi or config.RASTER_DPI
    max_px = max_px or config.RASTER_MAX_PX
    zoom = dpi / 72.0
    matrix = fitz.Matrix(zoom, zoom)
    pages: list[bytes] = []
    with fitz.open(stream=pdf_bytes, filetype="pdf") as doc:
        for page in doc:
            pix = page.get_pixmap(matrix=matrix, alpha=False)
            img = Image.open(io.BytesIO(pix.tobytes("png")))
            pages.append(_normalize_png(img, max_px))
    return pages


def image_to_png(image_bytes: bytes, max_px: int | None = None) -> bytes:
    """Load any Pillow-supported image and normalize to a clamped RGB PNG."""
    max_px = max_px or config.RASTER_MAX_PX
    img = Image.open(io.BytesIO(image_bytes))
    return _normalize_png(img, max_px)


def _normalize_png(img: Image.Image, max_px: int) -> bytes:
    if img.mode != "RGB":
        img = img.convert("RGB")
    w, h = img.size
    longest = max(w, h)
    if longest > max_px:
        scale = max_px / longest
        img = img.resize((round(w * scale), round(h * scale)), Image.LANCZOS)
    out = io.BytesIO()
    img.save(out, format="PNG")
    return out.getvalue()


def is_image(data: bytes) -> bool:
    """Cheap magic-byte sniff for the common invoice image formats."""
    return (
        data[:8] == b"\x89PNG\r\n\x1a\n"        # PNG
        or data[:3] == b"\xff\xd8\xff"          # JPEG
        or data[:4] in (b"II*\x00", b"MM\x00*")  # TIFF
        or data[:4] == b"RIFF"                   # WEBP (RIFF container)
    )
