"""Tests for postprocessing: locale number parsing, date parsing, total reconciliation.

These are the model-independent guarantees: we never trust the
model's arithmetic or number formatting — we re-parse and re-check here.
"""

from datetime import date
from decimal import Decimal

import pytest

from app.postprocess import (
    categorize_by_vendor,
    normalize_raw,
    parse_category,
    parse_decimal,
    parse_invoice_date,
    reconcile_totals,
)
from app.schema import Invoice, LineItem


# --- parse_decimal ---------------------------------------------------------

@pytest.mark.parametrize(
    "raw,expected",
    [
        ("1.250.000,00", Decimal("1250000.00")),   # Indonesian/European
        ("1,250,000.00", Decimal("1250000.00")),   # US
        ("1250000", Decimal("1250000")),           # plain
        ("1.250.000", Decimal("1250000")),         # European thousands, no decimals
        ("Rp 1.250.000", Decimal("1250000")),      # currency prefix + grouping
        ("$1,234.56", Decimal("1234.56")),         # symbol + US
        ("12,50", Decimal("12.50")),               # decimal comma, no grouping
        ("12.50", Decimal("12.50")),               # decimal dot, no grouping
        ("-50,00", Decimal("-50.00")),             # negative (credit line)
        ("  7 ", Decimal("7")),                    # whitespace
    ],
)
def test_parse_decimal_locale_formats(raw, expected):
    assert parse_decimal(raw) == expected


@pytest.mark.parametrize("raw", [None, "", "   ", "N/A", "-", "abc"])
def test_parse_decimal_unparseable_is_none(raw):
    assert parse_decimal(raw) is None


def test_parse_decimal_passthrough_numeric_types():
    assert parse_decimal(1250) == Decimal("1250")
    assert parse_decimal(Decimal("5.5")) == Decimal("5.5")


# --- parse_invoice_date ----------------------------------------------------

@pytest.mark.parametrize(
    "raw,expected",
    [
        ("21/06/2026", date(2026, 6, 21)),    # dayfirst (Indonesian convention)
        ("21-06-2026", date(2026, 6, 21)),
        ("2026-06-21", date(2026, 6, 21)),    # ISO
        ("21 Jun 2026", date(2026, 6, 21)),   # English abbrev
        ("21 Juni 2026", date(2026, 6, 21)),  # Indonesian month name
        ("21 Agustus 2025", date(2025, 8, 21)),
    ],
)
def test_parse_invoice_date_formats(raw, expected):
    assert parse_invoice_date(raw) == expected


@pytest.mark.parametrize("raw", [None, "", "not a date", "99/99/9999"])
def test_parse_invoice_date_unparseable_is_none(raw):
    assert parse_invoice_date(raw) is None


def test_parse_invoice_date_passthrough_date():
    assert parse_invoice_date(date(2026, 6, 21)) == date(2026, 6, 21)


# --- parse_category --------------------------------------------------------

@pytest.mark.parametrize(
    "raw,expected",
    [
        ("groceries", "groceries"),
        ("Groceries", "groceries"),       # case-normalized
        ("  MEDICAL ", "medical"),        # trimmed
        ("restaurant", "other"),          # off-list -> other
        ("", None),
        (None, None),
    ],
)
def test_parse_category_canonicalizes(raw, expected):
    assert parse_category(raw) == expected


def test_normalize_raw_coerces_category():
    out = normalize_raw({"category": "Shopping"})
    assert out["category"] == "shopping"


@pytest.mark.parametrize(
    "vendor,expected",
    [
        ("ALFAMART STA.KARET", "groceries"),
        ("PT INDOMARCO PRISMATAMA", "groceries"),   # legal-entity name still matches
        ("PT. Sumber Alfaria Trijaya, Tbk", "groceries"),
        ("Indomaret Cidatar", "groceries"),
        ("Lilac Beauty and Dental Clinic", "medical"),
        ("Warung Pasta @ Kemang", "dining"),
        ("SPBU Pertamina 34.567", "transport"),
        ("Toko Buku Gramedia", None),   # no rule -> None (fall back to model)
        (None, None),
    ],
)
def test_categorize_by_vendor(vendor, expected):
    assert categorize_by_vendor(vendor) == expected


def test_vendor_rule_overrides_model_category():
    # Model guessed "shopping" for a minimarket; the vendor rule corrects it.
    out = normalize_raw({"vendor_name": "Indomaret Cidatar", "category": "shopping"})
    assert out["category"] == "groceries"


def test_model_category_kept_when_no_vendor_rule():
    out = normalize_raw({"vendor_name": "Toko Buku Gramedia", "category": "shopping"})
    assert out["category"] == "shopping"


# --- normalize_raw ---------------------------------------------------------

def test_normalize_raw_produces_valid_invoice():
    raw = {
        "vendor_name": "PT Buah Segar",
        "invoice_number": "INV/2026/0042",
        "invoice_date": "21/06/2026",
        "currency": "IDR",
        "line_items": [
            {"description": "Mangga Harum Manis", "quantity": "10", "unit_price": "25.000", "amount": "250.000"},
        ],
        "subtotal": "250.000",
        "tax_amount": "27.500",
        "total_amount": "277.500",
    }
    inv = Invoice(**normalize_raw(raw))
    assert inv.invoice_date == date(2026, 6, 21)
    assert inv.total_amount == Decimal("277500")
    assert inv.line_items[0].amount == Decimal("250000")


def test_normalize_raw_keeps_absent_fields_null():
    raw = {"vendor_name": "X", "total_amount": "N/A", "invoice_date": ""}
    out = normalize_raw(raw)
    assert out["total_amount"] is None
    assert out["invoice_date"] is None


# --- reconcile_totals ------------------------------------------------------

def _invoice(**kw) -> Invoice:
    base = dict(
        line_items=[
            LineItem(description="A", amount=Decimal("100")),
            LineItem(description="B", amount=Decimal("200")),
        ],
        subtotal=Decimal("300"),
        tax_amount=Decimal("33"),
        total_amount=Decimal("333"),
    )
    base.update(kw)
    return Invoice(**base)


def test_reconcile_consistent_invoice():
    report = reconcile_totals(_invoice())
    assert report.line_items_sum == Decimal("300")
    assert report.subtotal_ok is True
    assert report.total_ok is True
    assert report.consistent is True


def test_reconcile_flags_total_mismatch():
    report = reconcile_totals(_invoice(total_amount=Decimal("999")))
    assert report.total_ok is False
    assert report.consistent is False
    assert report.notes  # explains what didn't add up


def test_reconcile_flags_subtotal_mismatch():
    report = reconcile_totals(_invoice(subtotal=Decimal("250")))
    assert report.subtotal_ok is False
    assert report.consistent is False


def test_reconcile_missing_data_is_not_an_inconsistency():
    # No line items, no subtotal -> nothing to contradict -> consistent.
    inv = Invoice(total_amount=Decimal("500"), line_items=[])
    report = reconcile_totals(inv)
    assert report.subtotal_ok is None
    assert report.consistent is True
