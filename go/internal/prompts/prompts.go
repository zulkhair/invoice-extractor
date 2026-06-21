// Package prompts builds the system/user prompts with the schema injected from
// schema.RawJSONSchema (all string/nullable), so the model never formats numbers.
package prompts

import (
	"strings"

	"invoice-extractor/internal/schema"
)

const systemTmpl = `You are a precise invoice data-extraction engine. You convert a single invoice into JSON that conforms EXACTLY to the schema below.

Hard rules:
- Output ONLY a single JSON object. No prose, no markdown, no code fences.
- If a field is not present on the invoice, return null. NEVER guess or fabricate.
- Copy values as printed. Do NOT reformat numbers or dates, do NOT do arithmetic, do NOT convert currencies. Downstream code normalizes formats and checks totals.
- For money fields, return the digits/grouping exactly as shown (e.g. "1.250.000,00").
- currency is the ISO 4217 code if determinable (e.g. IDR, USD), else null.
- line_items: one object per line on the invoice; omit (empty) if none.
- vendor_tax_id is the seller's tax number (NPWP for Indonesian invoices) if shown.

JSON schema (your output must validate against this):
%s`

const visionUser = `Read the invoice in the attached image and extract it into the JSON schema. Read the actual pixels — do not infer values from the schema. Remember: null for anything absent.`

func System() string {
	return strings.Replace(systemTmpl, "%s", schema.RawJSONSchema, 1)
}

func TextUser(documentText string) string {
	return "Extract the invoice below into the JSON schema. Remember: null for anything absent.\n\n" +
		"--- INVOICE TEXT ---\n" + documentText + "\n--- END INVOICE TEXT ---\n"
}

func VisionUser() string { return visionUser }
