"""System prompt + schema injection.

The schema block is generated from the Pydantic model (schema.py) so the prompt
and the validator can never drift. The system prompt states the hard rule:
never fabricate; null for absent fields.
"""

from __future__ import annotations

import json

from app.schema import CATEGORIES, invoice_json_schema

_SYSTEM = """\
You are a precise receipt/invoice extraction engine for a personal expense tracker. \
You convert a single receipt or invoice into JSON that conforms EXACTLY to the schema below.

Hard rules:
- Output ONLY a single JSON object. No prose, no markdown, no code fences.
- If a field is not present, return null. NEVER guess or fabricate.
- Copy values as printed. Do NOT reformat numbers or dates and do NOT do arithmetic.
- vendor_name: the store or business name — the place where the money was spent,
  usually printed at the top (not the legal "PT ..." entity unless that is all there is).
- transaction_datetime: the purchase date AND time, exactly as printed
  (e.g. "19-05-2017 06:51:42"). Include the time if the receipt shows one.
- currency: the ISO 4217 code (IDR for Indonesian receipts, USD, ...), else null.
- line_items: one object per PURCHASED item, with its name (description) and price
  (amount = the line total as printed, e.g. "25.000"). Do NOT include summary lines
  such as subtotal, discount, service, tax/PPN, total, cash, or change. Empty list if none.
- total_amount: leave null — it is computed downstream by summing the line item prices.
- category: classify the whole purchase into EXACTLY ONE of: {categories}. Guidance —
  groceries = food/drinks/daily needs from a minimarket or supermarket (Alfamart,
  Indomaret, etc.); dining = restaurants, cafes, food courts; medical = clinic,
  pharmacy, hospital, health; transport = fuel, ride-hailing, parking, tolls;
  utilities = electricity, water, internet, phone bills; shopping = non-food retail
  (clothes, electronics, household goods); entertainment = leisure; other = none fit.

JSON schema (your output must validate against this):
{schema}
"""

# Vision and text paths share the same contract; only the carrier differs.
# Trailing "/no_think" disables chain-of-thought on Qwen3 models (the Ollama `think`
# flag is ignored for them) — we want the JSON directly, and the reasoning otherwise
# burns the context budget and truncates the answer. Harmless for non-thinking models.
_TEXT_USER = """\
Extract the invoice below into the JSON schema. Remember: null for anything absent. /no_think

--- INVOICE TEXT ---
{document_text}
--- END INVOICE TEXT ---
"""

_VISION_USER = """\
Read the invoice in the attached image and extract it into the JSON schema. \
Read the actual pixels — do not infer values from the schema. \
Remember: null for anything absent. /no_think
"""


# OCR step (vision/OCR specialist, e.g. GLM-OCR). We want a faithful transcription,
# NOT interpretation — a general text model maps the result to the schema afterwards.
# OCR models are transcribers, not instruction-followers: feeding them the JSON schema
# makes them echo it, so keep this lean and transcription-focused.
_OCR = """\
OCR this receipt into Markdown. Transcribe exactly what is printed — do not summarize, \
reformat numbers, translate, or invent anything:
- the store / vendor name,
- the full transaction date and time,
- the currency or any "Rp"/"$" symbol if shown,
- a table of every printed line: each purchased item with its price, AND any
  subtotal / total / cash / change / tax (PPN) lines, exactly as printed.\
"""


def system_prompt() -> str:
    schema_str = json.dumps(invoice_json_schema(), indent=2)
    return _SYSTEM.format(schema=schema_str, categories=", ".join(CATEGORIES))


def ocr_prompt() -> str:
    return _OCR


def text_user_prompt(document_text: str) -> str:
    return _TEXT_USER.format(document_text=document_text)


def vision_user_prompt() -> str:
    return _VISION_USER
