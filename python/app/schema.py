"""Canonical output schema — the single source of truth.

The JSON schema the model is prompted with is generated from this model, and
every model response is validated against it. All fields are nullable: the model
must return null for absent fields, never fabricate.
"""

from __future__ import annotations

from datetime import date
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
    description: str
    quantity: Decimal | None = None
    unit_price: Decimal | None = None
    amount: Decimal | None = None


class Invoice(BaseModel):
    vendor_name: str | None = None
    vendor_tax_id: str | None = None          # NPWP for Indonesian invoices
    invoice_number: str | None = None
    invoice_date: date | None = None
    due_date: date | None = None
    currency: str | None = Field(default=None, description="ISO 4217, e.g. IDR")
    line_items: list[LineItem] = []
    subtotal: Decimal | None = None
    tax_amount: Decimal | None = None         # PPN where present
    total_amount: Decimal | None = None
    category: str | None = Field(
        default=None,
        description="Spending category, exactly one of: " + ", ".join(CATEGORIES),
    )
    raw_notes: str | None = None


# Fields we consider "required-ish": if the text path leaves any of these null,
# the pipeline falls back to the vision model. (Receipts often have no invoice
# number, and we don't track it — so it is not a fallback trigger.)
REQUIRED_ISH_FIELDS = ("vendor_name", "total_amount")


def invoice_json_schema() -> dict:
    """JSON schema injected into the prompt and (optionally) passed as Ollama's
    structured `format`. Generated from the Pydantic model so prompt and
    validation can never drift apart."""
    return Invoice.model_json_schema()
