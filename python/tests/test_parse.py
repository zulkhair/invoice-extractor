"""Tests for parsing raw model output into a validated Invoice.

The key guarantee: locale-formatted numbers survive JSON decoding. A model that
emits an amount as a bare JSON *number* (25.000) must not be silently truncated to
25.0 — we decode numbers as raw text so postprocess applies locale rules.
"""

from decimal import Decimal

from app.pipeline import parse_model_content


def test_bare_json_number_keeps_thousands_separator():
    # Model emitted amounts as JSON NUMBERS (not strings) with ID/EU grouping.
    # Naive json.loads would decode 25.000 -> 25.0; we must get 25000.
    content = '{"total_amount": 92.400, "subtotal": 80.000, "tax_amount": 8.400}'
    inv = parse_model_content(content)
    assert inv is not None
    assert inv.subtotal == Decimal("80000")
    assert inv.tax_amount == Decimal("8400")
    assert inv.total_amount == Decimal("92400")


def test_bare_json_number_line_items():
    content = (
        '{"line_items": ['
        '  {"description": "Cheezy Freezy M", "quantity": 1, "unit_price": 25.000, "amount": 25.000},'
        '  {"description": "Lemon Tea", "quantity": 1, "amount": 11.000}'
        ']}'
    )
    inv = parse_model_content(content)
    assert inv is not None
    assert [li.amount for li in inv.line_items] == [Decimal("25000"), Decimal("11000")]


def test_quoted_string_numbers_still_work():
    content = '{"total_amount": "1.250.000,00", "subtotal": "Rp 1.250.000"}'
    inv = parse_model_content(content)
    assert inv is not None
    assert inv.total_amount == Decimal("1250000.00")
    assert inv.subtotal == Decimal("1250000")


def test_genuine_decimal_amount_preserved():
    # A real fractional value (US-style) must stay fractional, not be grouped.
    content = '{"total_amount": 1234.56, "tax_amount": 12.50}'
    inv = parse_model_content(content)
    assert inv is not None
    assert inv.total_amount == Decimal("1234.56")
    assert inv.tax_amount == Decimal("12.50")


def test_plain_integer_amount():
    content = '{"total_amount": 5106000}'
    inv = parse_model_content(content)
    assert inv is not None
    assert inv.total_amount == Decimal("5106000")


def test_json_with_code_fence_and_prose():
    content = 'Here is the data:\n```json\n{"total_amount": 4.500}\n```\nDone.'
    inv = parse_model_content(content)
    assert inv is not None
    assert inv.total_amount == Decimal("4500")
