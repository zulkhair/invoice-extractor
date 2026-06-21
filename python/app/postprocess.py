"""Post-processing: turn the model's raw, locale-formatted strings into canonical
Decimal/date values, and re-check totals ourselves.

Core principle (spec Task 5): never trust the model's number formatting or
arithmetic. Parse locale formats deterministically here, then reconcile
line items vs subtotal vs total and flag inconsistencies.
"""

from __future__ import annotations

import re
from datetime import date, datetime
from decimal import Decimal, InvalidOperation

from dateutil import parser as date_parser
from pydantic import BaseModel

from app.schema import Invoice

# --- numbers ---------------------------------------------------------------

_NEG_RE = re.compile(r"-\s*\d")


def parse_decimal(value) -> Decimal | None:
    """Parse a locale-formatted money/number value into a canonical Decimal.

    Handles "1.250.000,00" (ID/EU), "1,250,000.00" (US), "Rp 1.250.000",
    "$1,234.56", "12,50", negatives, and currency symbols. Returns None for
    anything not parseable — never fabricate.
    """
    if value is None:
        return None
    if isinstance(value, Decimal):
        return value
    if isinstance(value, int):
        return Decimal(value)
    if isinstance(value, float):
        return Decimal(str(value))

    s = str(value).strip()
    if not s:
        return None

    neg = bool(_NEG_RE.search(s)) or (s.startswith("(") and s.endswith(")"))
    core = re.sub(r"[^0-9.,]", "", s)
    if not re.search(r"\d", core):
        return None

    core = _strip_separators(core)
    try:
        d = Decimal(core)
    except InvalidOperation:
        return None
    return -d if neg else d


def _strip_separators(core: str) -> str:
    has_dot = "." in core
    has_comma = "," in core
    if has_dot and has_comma:
        # the rightmost separator is the decimal point; the other is grouping
        if core.rfind(",") > core.rfind("."):
            return core.replace(".", "").replace(",", ".")
        return core.replace(",", "")
    if has_comma:
        return _resolve_single(core, ",")
    if has_dot:
        return _resolve_single(core, ".")
    return core


def _resolve_single(core: str, sep: str) -> str:
    parts = core.split(sep)
    if len(parts) > 2:
        # several occurrences -> grouping separator
        return core.replace(sep, "")
    left, right = parts
    # one separator, three trailing digits -> grouping ("1.250" -> 1250);
    # otherwise it's the decimal point ("12,50" -> 12.50)
    if len(right) == 3 and len(left) >= 1:
        return left + right
    return left + "." + right


# --- dates -----------------------------------------------------------------

_ID_MONTHS = {
    "januari": "january", "februari": "february", "maret": "march",
    "april": "april", "mei": "may", "juni": "june", "juli": "july",
    "agustus": "august", "september": "september", "oktober": "october",
    "november": "november", "desember": "december",
}


def parse_invoice_date(value) -> date | None:
    """Parse a date string into a date. Day-first (ID convention) and tolerant
    of Indonesian month names. Returns None if unparseable."""
    if value is None:
        return None
    if isinstance(value, datetime):
        return value.date()
    if isinstance(value, date):
        return value
    s = str(value).strip()
    if not s:
        return None
    low = s.lower()
    for idm, en in _ID_MONTHS.items():
        if idm in low:
            low = low.replace(idm, en)
    try:
        dt = date_parser.parse(low, dayfirst=True, fuzzy=False)
    except (ValueError, OverflowError, TypeError):
        return None
    return dt.date()


# --- raw normalization -----------------------------------------------------

_MONEY_FIELDS = ("subtotal", "tax_amount", "total_amount")
_DATE_FIELDS = ("invoice_date", "due_date")
_LINE_MONEY = ("quantity", "unit_price", "amount")


def normalize_raw(raw: dict) -> dict:
    """Normalize a raw model dict into types the Invoice schema accepts.

    Locale numbers -> Decimal, date strings -> date, unparseable -> None.
    Returns a new dict; does not mutate the input.
    """
    out = dict(raw)
    for f in _MONEY_FIELDS:
        if f in out:
            out[f] = parse_decimal(out[f])
    for f in _DATE_FIELDS:
        if f in out:
            out[f] = parse_invoice_date(out[f])
    items = out.get("line_items")
    if isinstance(items, list):
        norm_items = []
        for it in items:
            if not isinstance(it, dict):
                continue
            it = dict(it)
            for f in _LINE_MONEY:
                if f in it:
                    it[f] = parse_decimal(it[f])
            norm_items.append(it)
        out["line_items"] = norm_items
    return out


# --- total reconciliation --------------------------------------------------

class ConsistencyReport(BaseModel):
    line_items_sum: Decimal | None = None
    subtotal_ok: bool | None = None      # None = couldn't check (data missing)
    total_ok: bool | None = None
    consistent: bool = True              # False only when a check actually failed
    notes: list[str] = []


def reconcile_totals(
    invoice: Invoice, tolerance: Decimal = Decimal("0.02")
) -> ConsistencyReport:
    """Recompute totals from line items and cross-check subtotal/tax/total.

    A check that can't run (missing data) is recorded as None and does NOT make
    the invoice inconsistent — only a check that runs and fails does.
    """
    notes: list[str] = []

    amounts = [li.amount for li in invoice.line_items if li.amount is not None]
    items_sum = sum(amounts, Decimal("0")) if amounts else None

    subtotal_ok: bool | None = None
    if items_sum is not None and invoice.subtotal is not None:
        subtotal_ok = abs(items_sum - invoice.subtotal) <= tolerance
        if not subtotal_ok:
            notes.append(f"line items sum {items_sum} != subtotal {invoice.subtotal}")

    total_ok: bool | None = None
    base = invoice.subtotal if invoice.subtotal is not None else items_sum
    if invoice.total_amount is not None and base is not None:
        expected = base + (invoice.tax_amount or Decimal("0"))
        total_ok = abs(expected - invoice.total_amount) <= tolerance
        if not total_ok:
            notes.append(
                f"subtotal+tax {expected} != total {invoice.total_amount}"
            )

    consistent = subtotal_ok is not False and total_ok is not False
    return ConsistencyReport(
        line_items_sum=items_sum,
        subtotal_ok=subtotal_ok,
        total_ok=total_ok,
        consistent=consistent,
        notes=notes,
    )
