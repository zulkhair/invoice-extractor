"""Tests for postprocessing: locale number parsing, datetime parsing, category
assignment, and computing the total by summing line item prices.

These are the model-independent guarantees: we never trust the model's number
formatting or its grand total — we re-parse and re-sum here.
"""

from datetime import datetime
from decimal import Decimal

import pytest

from app.postprocess import (
    categorize_by_vendor,
    normalize_raw,
    parse_category,
    parse_datetime,
    parse_decimal,
)
from app.schema import Invoice


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
        ("12,50", Decimal("12.50")),               # decimal comma
        ("12.50", Decimal("12.50")),               # decimal dot
        ("-50,00", Decimal("-50.00")),             # negative
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


# --- parse_datetime --------------------------------------------------------

@pytest.mark.parametrize(
    "raw,expected",
    [
        ("19-05-2017 06:51:42", datetime(2017, 5, 19, 6, 51, 42)),  # dayfirst + time
        ("20 Mar 2015 08:20", datetime(2015, 3, 20, 8, 20)),
        ("16-11-15 16:04:52", datetime(2015, 11, 16, 16, 4, 52)),
        ("21/06/2026", datetime(2026, 6, 21, 0, 0)),                # date only -> midnight
        ("21 Juni 2026 14:30", datetime(2026, 6, 21, 14, 30)),      # Indonesian month
    ],
)
def test_parse_datetime_formats(raw, expected):
    assert parse_datetime(raw) == expected


@pytest.mark.parametrize("raw", [None, "", "not a date"])
def test_parse_datetime_unparseable_is_none(raw):
    assert parse_datetime(raw) is None


def test_parse_datetime_passthrough():
    dt = datetime(2026, 6, 21, 9, 0)
    assert parse_datetime(dt) == dt


# --- category --------------------------------------------------------------

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


@pytest.mark.parametrize(
    "vendor,expected",
    [
        ("ALFAMART STA.KARET", "groceries"),
        ("PT INDOMARCO PRISMATAMA", "groceries"),       # legal-entity name matches
        ("PT. Sumber Alfaria Trijaya, Tbk", "groceries"),
        ("Indomaret Cidatar", "groceries"),
        ("Lilac Beauty and Dental Clinic", "medical"),
        ("Warung Pasta @ Kemang", "dining"),
        ("SPBU Pertamina 34.567", "transport"),
        ("Toko Buku Gramedia", None),     # no rule -> None (fall back to model)
        (None, None),
    ],
)
def test_categorize_by_vendor(vendor, expected):
    assert categorize_by_vendor(vendor) == expected


def test_vendor_rule_overrides_model_category():
    out = normalize_raw({"vendor_name": "Indomaret Cidatar", "category": "shopping"})
    assert out["category"] == "groceries"


def test_model_category_kept_when_no_vendor_rule():
    out = normalize_raw({"vendor_name": "Toko Buku Gramedia", "category": "shopping"})
    assert out["category"] == "shopping"


# --- normalize_raw + computed total ----------------------------------------

def test_normalize_raw_produces_valid_invoice():
    raw = {
        "vendor_name": "Warung Pasta @ Kemang",
        "transaction_datetime": "16-11-15 16:04:52",
        "currency": "IDR",
        "line_items": [
            {"description": "Cheezy Freezy M", "amount": "25.000"},
            {"description": "Lemon Tea", "amount": "11.000"},
        ],
    }
    inv = Invoice(**normalize_raw(raw))
    assert inv.transaction_datetime == datetime(2015, 11, 16, 16, 4, 52)
    assert inv.category == "dining"                         # vendor rule: "warung"
    assert [li.amount for li in inv.line_items] == [Decimal("25000"), Decimal("11000")]
    assert inv.total_amount == Decimal("36000")            # computed sum


def test_total_is_summed_not_taken_from_model():
    # The model's own total (often the cash tendered) is ignored; we sum the items.
    out = normalize_raw({
        "line_items": [{"description": "A", "amount": "4.500"}],
        "total_amount": "5.000",
    })
    assert out["total_amount"] == Decimal("4500")


def test_normalize_raw_no_items_total_none():
    out = normalize_raw({"vendor_name": "X", "line_items": []})
    assert out["total_amount"] is None
