"""Tests for field-level accuracy scoring (the benchmark harness core)."""

from datetime import date
from decimal import Decimal

from app.schema import Invoice, LineItem
from app.scoring import score_invoice


def _gold() -> Invoice:
    return Invoice(
        vendor_name="PT Buah Segar",
        invoice_number="INV/2026/0042",
        invoice_date=date(2026, 6, 21),
        currency="IDR",
        total_amount=Decimal("277500"),
        category="groceries",
        line_items=[
            LineItem(description="Mangga Harum Manis", amount=Decimal("250000")),
            LineItem(description="Jeruk", amount=Decimal("27500")),
        ],
    )


def test_perfect_match_scores_full():
    s = score_invoice(_gold(), _gold())
    assert s.fields_correct == s.fields_total
    assert s.accuracy == 1.0
    assert s.line_item_recall == 1.0
    assert s.line_item_precision == 1.0


def test_wrong_scalar_field_counts_against():
    pred = _gold().model_copy(update={"total_amount": Decimal("999")})
    s = score_invoice(pred, _gold())
    assert s.fields_correct == s.fields_total - 1
    assert s.per_field["total_amount"] is False
    assert s.per_field["vendor_name"] is True


def test_string_match_is_whitespace_and_case_insensitive():
    pred = _gold().model_copy(update={"vendor_name": "  pt  buah segar "})
    s = score_invoice(pred, _gold())
    assert s.per_field["vendor_name"] is True


def test_missing_line_item_lowers_recall_not_precision():
    pred = _gold().model_copy(
        update={"line_items": [LineItem(description="Mangga Harum Manis", amount=Decimal("250000"))]}
    )
    s = score_invoice(pred, _gold())
    assert s.line_item_recall == 0.5
    assert s.line_item_precision == 1.0


def test_wrong_category_counts_against():
    pred = _gold().model_copy(update={"category": "dining"})
    s = score_invoice(pred, _gold())
    assert s.per_field["category"] is False
    assert s.per_field["vendor_name"] is True


def test_null_vs_value_is_incorrect_but_null_vs_null_is_correct():
    gold = Invoice(vendor_name="X", category=None)
    pred = Invoice(vendor_name=None, category=None)
    s = score_invoice(pred, gold)
    assert s.per_field["vendor_name"] is False     # gold had a value, pred null
    assert s.per_field["category"] is True         # both null
