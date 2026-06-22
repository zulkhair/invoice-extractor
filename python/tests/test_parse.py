"""Tests for parsing raw model output into a validated Invoice.

Two guarantees: locale-formatted numbers survive JSON decoding (a bare 25.000 must
not become 25.0), and total_amount is the computed sum of the line item prices,
never whatever grand total the model printed.
"""

from decimal import Decimal

from app.pipeline import parse_model_content


def test_bare_json_number_line_item_keeps_thousands():
    # Amounts emitted as JSON NUMBERS with ID/EU grouping must not be truncated:
    # naive json.loads would read 25.000 as 25.0.
    content = (
        '{"line_items": ['
        '  {"description": "Cheezy Freezy M", "amount": 25.000},'
        '  {"description": "Lemon Tea", "amount": 11.000}'
        ']}'
    )
    inv = parse_model_content(content)
    assert inv is not None
    assert [li.amount for li in inv.line_items] == [Decimal("25000"), Decimal("11000")]
    assert inv.total_amount == Decimal("36000")          # computed sum


def test_quoted_string_amounts_still_work():
    content = '{"line_items": [{"description": "X", "amount": "1.250.000,00"}]}'
    inv = parse_model_content(content)
    assert inv is not None
    assert inv.line_items[0].amount == Decimal("1250000.00")
    assert inv.total_amount == Decimal("1250000.00")


def test_genuine_decimal_amount_preserved():
    content = '{"line_items": [{"description": "X", "amount": 1234.56}]}'
    inv = parse_model_content(content)
    assert inv is not None
    assert inv.line_items[0].amount == Decimal("1234.56")


def test_total_uses_sum_not_model_total():
    # The model printed a grand total (the cash tendered); we sum the items instead.
    content = '{"total_amount": 5.000, "line_items": [{"description": "Bread", "amount": 4.500}]}'
    inv = parse_model_content(content)
    assert inv is not None
    assert inv.total_amount == Decimal("4500")


def test_json_with_code_fence_and_prose():
    content = 'Here:\n```json\n{"line_items": [{"description": "Bread", "amount": 4.500}]}\n```\nDone.'
    inv = parse_model_content(content)
    assert inv is not None
    assert inv.total_amount == Decimal("4500")
