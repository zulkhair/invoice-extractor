"""Build a synthetic Indonesian-style invoice fixture (digital PDF + rasterized
'scan' PNG) plus matching ground-truth labels.

Lets the text-path (Task 3) and vision-path (Task 4) gates run without real,
private invoices. Real labeled invoices still drive the actual benchmark.
"""

from __future__ import annotations

import json
from pathlib import Path

# Canonical ground truth. Money/date strings are in canonical form so that
# json.load -> Invoice(**label) validates directly.
LABEL = {
    "vendor_name": "PT Buah Segar Nusantara",
    "vendor_tax_id": "01.234.567.8-901.000",
    "invoice_number": "INV/2026/06/0042",
    "invoice_date": "2026-06-21",
    "due_date": "2026-07-21",
    "currency": "IDR",
    "line_items": [
        {"description": "Mangga Harum Manis", "quantity": "100", "unit_price": "25000", "amount": "2500000"},
        {"description": "Jeruk Medan", "quantity": "50", "unit_price": "18000", "amount": "900000"},
        {"description": "Apel Fuji", "quantity": "40", "unit_price": "30000", "amount": "1200000"},
    ],
    "subtotal": "4600000",
    "tax_amount": "506000",
    "total_amount": "5106000",
}

# What's printed on the page (locale-formatted, the way a real invoice looks).
_LINES = [
    ("PT Buah Segar Nusantara", 16),
    ("Jl. Pasar Induk No. 7, Jakarta Timur", 10),
    ("NPWP: 01.234.567.8-901.000", 10),
    ("", 6),
    ("FAKTUR / INVOICE", 13),
    ("No. Faktur : INV/2026/06/0042", 11),
    ("Tanggal    : 21/06/2026", 11),
    ("Jatuh Tempo: 21/07/2026", 11),
    ("Mata Uang  : IDR", 11),
    ("", 6),
    ("Deskripsi                        Qty     Harga        Jumlah", 10),
    ("Mangga Harum Manis               100     25.000       2.500.000", 10),
    ("Jeruk Medan                       50     18.000         900.000", 10),
    ("Apel Fuji                         40     30.000       1.200.000", 10),
    ("", 6),
    ("Subtotal                                            4.600.000", 10),
    ("PPN 11%                                               506.000", 10),
    ("Total                                               5.106.000", 11),
]


def build_synthetic(fixtures_dir: Path, labels_dir: Path) -> dict[str, Path]:
    """Create the PDF, PNG, and label files. Returns the paths."""
    from reportlab.lib.pagesizes import A4
    from reportlab.pdfgen import canvas

    fixtures_dir.mkdir(parents=True, exist_ok=True)
    labels_dir.mkdir(parents=True, exist_ok=True)

    pdf_path = fixtures_dir / "synthetic_invoice.pdf"
    png_path = fixtures_dir / "synthetic_invoice_scan.png"

    # --- digital PDF with a real text layer ---
    c = canvas.Canvas(str(pdf_path), pagesize=A4)
    _width, height = A4
    y = height - 60
    for text, size in _LINES:
        if text:
            c.setFont("Courier", size)
            c.drawString(50, y, text)
        y -= size + 8
    c.showPage()
    c.save()

    # --- rasterized 'scan' PNG (vision path / bench) ---
    from app.rasterize import pdf_to_pngs

    pngs = pdf_to_pngs(pdf_path.read_bytes())
    png_path.write_bytes(pngs[0])

    # --- labels (same content for both fixtures) ---
    (labels_dir / "synthetic_invoice.json").write_text(json.dumps(LABEL, indent=2))
    (labels_dir / "synthetic_invoice_scan.json").write_text(json.dumps(LABEL, indent=2))

    return {"pdf": pdf_path, "png": png_path}
