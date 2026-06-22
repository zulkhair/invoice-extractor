"""Smoke tests for the document layer (PyMuPDF text detection + rasterize) using
the generated synthetic invoice. No Ollama required."""

from decimal import Decimal

from app.pdf_text import extract_text_layer, is_pdf
from app.rasterize import is_image, pdf_to_pngs
from app.schema import Invoice


def test_synthetic_pdf_has_text_layer(synthetic_pdf_bytes):
    assert is_pdf(synthetic_pdf_bytes)
    layer = extract_text_layer(synthetic_pdf_bytes)
    assert layer.has_text_layer
    assert layer.page_count == 1
    assert "Buah Segar" in layer.text
    assert "INV/2026/06/0042" in layer.text


def test_synthetic_pdf_rasterizes_to_png(synthetic_pdf_bytes):
    pngs = pdf_to_pngs(synthetic_pdf_bytes)
    assert len(pngs) == 1
    assert is_image(pngs[0])
    assert pngs[0][:8] == b"\x89PNG\r\n\x1a\n"


def test_ground_truth_label_is_schema_valid(synthetic):
    inv = Invoice(**synthetic["label"])
    assert inv.total_amount == Decimal("4600000")     # sum of the 3 line items
    assert inv.category == "groceries"
    assert inv.transaction_datetime is not None
    assert len(inv.line_items) == 3
