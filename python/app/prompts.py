"""System prompt + schema injection.

The schema block is generated from the Pydantic model (schema.py) so the prompt
and the validator can never drift. The system prompt states the hard rule from
the spec: never fabricate; null for absent fields.
"""

from __future__ import annotations

import json

from app.schema import invoice_json_schema

_SYSTEM = """\
You are a precise invoice data-extraction engine. You convert a single invoice \
into JSON that conforms EXACTLY to the schema below.

Hard rules:
- Output ONLY a single JSON object. No prose, no markdown, no code fences.
- If a field is not present on the invoice, return null. NEVER guess or fabricate.
- Copy values as printed. Do NOT reformat numbers or dates, do NOT do arithmetic,
  do NOT convert currencies. Downstream code normalizes formats and checks totals.
- For money fields, return the digits/grouping exactly as shown (e.g. "1.250.000,00").
- currency is the ISO 4217 code if determinable (e.g. IDR, USD), else null.
- line_items: one object per line on the invoice; omit the list (empty) if none.
- vendor_tax_id is the seller's tax number (NPWP for Indonesian invoices) if shown.

JSON schema (your output must validate against this):
{schema}
"""

# Vision and text paths share the same contract; only the carrier differs.
_TEXT_USER = """\
Extract the invoice below into the JSON schema. Remember: null for anything absent.

--- INVOICE TEXT ---
{document_text}
--- END INVOICE TEXT ---
"""

_VISION_USER = """\
Read the invoice in the attached image and extract it into the JSON schema. \
Read the actual pixels — do not infer values from the schema. \
Remember: null for anything absent.
"""


def system_prompt() -> str:
    schema_str = json.dumps(invoice_json_schema(), indent=2)
    return _SYSTEM.format(schema=schema_str)


def text_user_prompt(document_text: str) -> str:
    return _TEXT_USER.format(document_text=document_text)


def vision_user_prompt() -> str:
    return _VISION_USER
