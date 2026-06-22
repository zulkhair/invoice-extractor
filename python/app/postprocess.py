"""Post-processing: turn the model's raw, locale-formatted strings into canonical
Decimal/datetime values, classify the category, and compute the total ourselves.

Core principle: never trust the model's number formatting or arithmetic. Parse
locale formats and dates deterministically here, and compute the total by summing
the line item prices rather than trusting whatever total the model printed.
"""

from __future__ import annotations

import re
from datetime import date, datetime
from decimal import Decimal, InvalidOperation

from dateutil import parser as date_parser

from app.schema import CATEGORIES

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


def parse_datetime(value) -> datetime | None:
    """Parse a transaction date+time. Day-first (ID convention), tolerant of
    Indonesian month names and messy receipt formats. Keeps the time when present
    (midnight otherwise). Returns None if unparseable."""
    if value is None:
        return None
    if isinstance(value, datetime):
        return value
    if isinstance(value, date):
        return datetime(value.year, value.month, value.day)
    s = str(value).strip()
    if not s:
        return None
    low = s.lower()
    for idm, en in _ID_MONTHS.items():
        if idm in low:
            low = low.replace(idm, en)
    try:
        return date_parser.parse(low, dayfirst=True, fuzzy=True)
    except (ValueError, OverflowError, TypeError):
        return None


# --- category --------------------------------------------------------------

def parse_category(value) -> str | None:
    """Canonicalize a spending category to the known vocabulary.

    Lowercase + trim; anything off-list becomes "other" so the tracker only ever
    sees the fixed buckets. Empty/None stays None (model did not classify).
    """
    if value is None:
        return None
    s = str(value).strip().lower()
    if not s:
        return None
    return s if s in CATEGORIES else "other"


# Vendor-keyword -> category. A deterministic rule is far more reliable than the
# small model for known Indonesian chains/keywords (the model insists Alfamart is
# "shopping", etc.), so a match here overrides the model's guess. First match wins;
# extend the keyword lists for your own vendors.
_VENDOR_CATEGORY_RULES = (
    ("medical", ("apotek", "apotik", "pharma", "farma", "klinik", "clinic", "hospital",
                 "rumah sakit", "dental", "dokter", "medika", "optik")),
    ("groceries", ("alfamart", "alfaria", "alfamidi", "indomaret", "indomarco",
                   "supermarket", "minimarket", "superindo", "hypermart", "transmart",
                   "giant", "lawson", "circle k", "familymart", "grosir", "swalayan")),
    ("dining", ("resto", "restaurant", "rumah makan", "warung", "warteg", "cafe", "kafe",
                "coffee", "kopi", "kedai", "bakery", "pizza", "kfc", "mcdonald", "burger",
                "bakso", "kitchen")),
    ("transport", ("pertamina", "spbu", "gojek", "grab", "parkir", "parking",
                   "transjakarta", "kereta", "krl", "bensin")),
    ("utilities", ("pln", "telkom", "indihome", "pdam", "biznet", "listrik", "pulsa")),
)


def categorize_by_vendor(vendor_name) -> str | None:
    """Best-effort category from the vendor name via keyword rules; None if no
    rule fires (caller then falls back to the model's own category guess)."""
    if not vendor_name:
        return None
    low = str(vendor_name).lower()
    for category, keywords in _VENDOR_CATEGORY_RULES:
        if any(kw in low for kw in keywords):
            return category
    return None


# --- raw normalization -----------------------------------------------------

def normalize_raw(raw: dict) -> dict:
    """Normalize a raw model dict into types the Invoice schema accepts.

    Parses the datetime and each line item price (locale -> Decimal), assigns the
    category (vendor rule overriding the model), and computes total_amount by summing
    the line item amounts — the model's own total is ignored. Returns a new dict;
    does not mutate the input.
    """
    out = dict(raw)

    if "transaction_datetime" in out:
        out["transaction_datetime"] = parse_datetime(out["transaction_datetime"])

    # Category: the vendor-keyword rule overrides the model's guess when it fires
    # (reliable for known chains); otherwise keep the model's canonicalized category.
    out["category"] = (
        categorize_by_vendor(out.get("vendor_name")) or parse_category(out.get("category"))
    )

    # Line items: parse each price, then compute the total ourselves (never trust
    # the model's printed grand total — it often grabs the cash tendered).
    total: Decimal | None = None
    items = out.get("line_items")
    if isinstance(items, list):
        norm_items = []
        for it in items:
            if not isinstance(it, dict):
                continue
            it = dict(it)
            if "amount" in it:
                it["amount"] = parse_decimal(it["amount"])
            norm_items.append(it)
        out["line_items"] = norm_items
        amounts = [it["amount"] for it in norm_items if it.get("amount") is not None]
        if amounts:
            total = sum(amounts, Decimal("0"))
    out["total_amount"] = total
    return out
