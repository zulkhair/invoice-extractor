"""Canonical output schema — the single source of truth.

The JSON schema the model is prompted with is generated from this model, and
every model response is validated against it. All fields are nullable: the model
must return null for absent fields, never fabricate.
"""

from __future__ import annotations

from datetime import datetime
from decimal import Decimal

from pydantic import BaseModel, Field

# Spending categories for the expense tracker. The model classifies each receipt
# into exactly one of these; postprocess coerces anything off-list to "other".
CATEGORIES = (
    "groceries",
    "dining",
    "medical",
    "transport",
    "utilities",
    "shopping",
    "entertainment",
    "other",
)


class LineItem(BaseModel):
    description: str                       # item name, as printed
    amount: Decimal | None = None          # line price (the line total, as printed)


class Invoice(BaseModel):
    """An expense record for a personal spending tracker: who / when / what / how-much
    + a category. `total_amount` is NOT taken from the model — it is computed downstream
    by summing the line item amounts (see postprocess.normalize_raw)."""

    vendor_name: str | None = None
    transaction_datetime: datetime | None = None   # purchase date + time
    currency: str | None = Field(default=None, description="ISO 4217, e.g. IDR")
    category: str | None = Field(
        default=None,
        description="Spending category, exactly one of: " + ", ".join(CATEGORIES),
    )
    line_items: list[LineItem] = []
    total_amount: Decimal | None = None     # computed = sum of line item amounts


# Fields we consider "required-ish": if the text path leaves any of these null, the
# pipeline falls back to the vision model. total_amount exists only once line items
# were extracted, so this also catches an empty extraction.
REQUIRED_ISH_FIELDS = ("vendor_name", "total_amount")


def invoice_json_schema() -> dict:
    """JSON schema injected into the prompt and (optionally) passed as Ollama's
    structured `format`. Generated from the Pydantic model so prompt and
    validation can never drift apart."""
    return Invoice.model_json_schema()
