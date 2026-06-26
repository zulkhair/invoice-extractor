// Package prompts builds the system/user prompts with the schema injected from
// schema.RawJSONSchema (all string/nullable), so the model never formats numbers.
package prompts

import (
	"strings"

	"invoice-extractor/internal/schema"
)

const systemTmpl = `You are a precise receipt/invoice extraction engine for a personal expense tracker. You convert a single receipt or invoice into JSON that conforms EXACTLY to the schema below.

Hard rules:
- Output ONLY a single JSON object. No prose, no markdown, no code fences.
- If a field is not present, return null. NEVER guess or fabricate.
- Copy values as printed. Do NOT reformat numbers or dates and do NOT do arithmetic.
- vendor_name: the store or business name — the place where the money was spent, usually printed at the top (not the legal "PT ..." entity unless that is all there is).
- transaction_datetime: the purchase date AND time, exactly as printed (e.g. "19-05-2017 06:51:42"). Include the time if the receipt shows one.
- currency: the ISO 4217 code (IDR for Indonesian receipts, USD, ...), else null.
- line_items: one object per PURCHASED item, with its name (description) and price (amount = the line total as printed, e.g. "25.000"). Do NOT include summary lines such as subtotal, discount, service, tax/PPN, total, cash, or change. Empty list if none.
- total_amount: leave null — it is computed downstream by summing the line item prices.
- category: classify the whole purchase into EXACTLY ONE of: groceries, dining, medical, transport, utilities, shopping, entertainment, other. Guidance — groceries = food/drinks/daily needs from a minimarket or supermarket (Alfamart, Indomaret, etc.); dining = restaurants, cafes, food courts; medical = clinic, pharmacy, hospital, health; transport = fuel, ride-hailing, parking, tolls; utilities = electricity, water, internet, phone bills; shopping = non-food retail (clothes, electronics, household goods); entertainment = leisure; other = none fit.

JSON schema (your output must validate against this):
%s`

// Trailing "/no_think" disables chain-of-thought on Qwen3 models (the Ollama think
// flag is ignored for them). Harmless for non-thinking models like qwen2.5 / glm-ocr.
const visionUser = `Read the invoice in the attached image and extract it into the JSON schema. Read the actual pixels — do not infer values from the schema. Remember: null for anything absent. /no_think`

// ocrText asks a dedicated OCR model (e.g. GLM-OCR) for a faithful transcription, NOT
// interpretation — a general text model maps the result to the schema afterwards. OCR
// models echo the schema if you feed it, so this stays lean and transcription-focused.
const ocrText = `OCR this receipt into Markdown. Transcribe exactly what is printed — do not summarize, reformat numbers, translate, or invent anything:
- the store / vendor name,
- the full transaction date and time,
- the currency or any "Rp"/"$" symbol if shown,
- a table of every printed line: each purchased item with its price, AND any subtotal / total / cash / change / tax (PPN) lines, exactly as printed.`

func System() string {
	return strings.Replace(systemTmpl, "%s", schema.RawJSONSchema, 1)
}

func TextUser(documentText string) string {
	return "Extract the invoice below into the JSON schema. Remember: null for anything absent. /no_think\n\n" +
		"--- INVOICE TEXT ---\n" + documentText + "\n--- END INVOICE TEXT ---\n"
}

func VisionUser() string { return visionUser }

// OCR is the prompt for the dedicated OCR model in the two-model vision path.
func OCR() string { return ocrText }
