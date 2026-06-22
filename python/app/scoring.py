"""Field-level accuracy scoring: compare a predicted Invoice to ground truth.

Used by scripts/bench.py (the decision tool) and tests/test_scoring.py. Scalar
fields are scored exact-match (strings normalized for whitespace/case); line items
get precision/recall via greedy description+amount matching.
"""

from __future__ import annotations

from pydantic import BaseModel

from app.schema import Invoice, LineItem

# Scalar fields scored for exact match (line_items scored separately). The expense
# tracker keeps only what/where/when/how-much + category; total_amount is the computed
# sum of line item prices.
SCALAR_FIELDS = (
    "vendor_name",
    "transaction_datetime",
    "currency",
    "category",
    "total_amount",
)


class InvoiceScore(BaseModel):
    fields_total: int
    fields_correct: int
    accuracy: float
    per_field: dict[str, bool]
    line_item_precision: float
    line_item_recall: float
    line_item_f1: float


def score_invoice(pred: Invoice, gold: Invoice) -> InvoiceScore:
    per_field = {f: _scalar_eq(getattr(pred, f), getattr(gold, f)) for f in SCALAR_FIELDS}
    total = len(SCALAR_FIELDS)
    correct = sum(per_field.values())
    p, r, f1 = _score_line_items(pred.line_items, gold.line_items)
    return InvoiceScore(
        fields_total=total,
        fields_correct=correct,
        accuracy=correct / total if total else 1.0,
        per_field=per_field,
        line_item_precision=p,
        line_item_recall=r,
        line_item_f1=f1,
    )


def _norm(s: str) -> str:
    return " ".join(s.split()).casefold()


def _scalar_eq(a, b) -> bool:
    if a is None and b is None:
        return True
    if a is None or b is None:
        return False
    if isinstance(a, str) and isinstance(b, str):
        return _norm(a) == _norm(b)
    return a == b


def _line_eq(p: LineItem, g: LineItem) -> bool:
    return _norm(p.description or "") == _norm(g.description or "") and p.amount == g.amount


def _score_line_items(
    pred: list[LineItem], gold: list[LineItem]
) -> tuple[float, float, float]:
    if not pred and not gold:
        return 1.0, 1.0, 1.0
    used: set[int] = set()
    matched = 0
    for g in gold:
        for i, p in enumerate(pred):
            if i in used:
                continue
            if _line_eq(p, g):
                used.add(i)
                matched += 1
                break
    precision = matched / len(pred) if pred else 0.0
    recall = matched / len(gold) if gold else 0.0
    f1 = (
        2 * precision * recall / (precision + recall)
        if (precision + recall) > 0
        else 0.0
    )
    return precision, recall, f1
