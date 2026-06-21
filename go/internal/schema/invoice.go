// Package schema defines the two-struct wire/canonical pattern (spec Section 5).
//
// RawInvoice is what the model emits: every numeric/date field is a *string so a
// malformed value (e.g. "1.250.000,00" or "21 Juni 2026") never breaks the whole
// unmarshal. postprocess coerces RawInvoice -> Invoice (typed, validated).
package schema

import (
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/shopspring/decimal"
)

// --- wire (lenient) ---

type RawLineItem struct {
	Description string  `json:"description"`
	Quantity    *string `json:"quantity"`
	UnitPrice   *string `json:"unit_price"`
	Amount      *string `json:"amount"`
}

type RawInvoice struct {
	VendorName    *string       `json:"vendor_name"`
	VendorTaxID   *string       `json:"vendor_tax_id"` // NPWP
	InvoiceNumber *string       `json:"invoice_number"`
	InvoiceDate   *string       `json:"invoice_date"`
	DueDate       *string       `json:"due_date"`
	Currency      *string       `json:"currency"` // ISO 4217, e.g. IDR
	LineItems     []RawLineItem `json:"line_items"`
	Subtotal      *string       `json:"subtotal"`
	TaxAmount     *string       `json:"tax_amount"` // PPN
	TotalAmount   *string       `json:"total_amount"`
	RawNotes      *string       `json:"raw_notes"`
}

// --- canonical (typed, validated) ---

type LineItem struct {
	Description string           `json:"description"`
	Quantity    *decimal.Decimal `json:"quantity,omitempty"`
	UnitPrice   *decimal.Decimal `json:"unit_price,omitempty"`
	Amount      *decimal.Decimal `json:"amount,omitempty"`
}

type Invoice struct {
	VendorName    *string          `json:"vendor_name"`
	VendorTaxID   *string          `json:"vendor_tax_id"`
	InvoiceNumber *string          `json:"invoice_number" validate:"omitempty"`
	InvoiceDate   *time.Time       `json:"invoice_date"`
	DueDate       *time.Time       `json:"due_date"`
	Currency      *string          `json:"currency" validate:"omitempty,len=3"`
	LineItems     []LineItem       `json:"line_items"`
	Subtotal      *decimal.Decimal `json:"subtotal"`
	TaxAmount     *decimal.Decimal `json:"tax_amount"`
	TotalAmount   *decimal.Decimal `json:"total_amount"`
	RawNotes      *string          `json:"raw_notes"`

	// Pipeline metadata (not from the model).
	Consistent bool `json:"consistent"`
}

var validate = validator.New()

// Validate runs struct validation on the canonical invoice.
func (i *Invoice) Validate() error { return validate.Struct(i) }

// HasRequiredish reports whether vendor, invoice number, and total are all
// present. The text path falls back to vision when this is false (spec Task 4).
func (i *Invoice) HasRequiredish() bool {
	return nonEmpty(i.VendorName) && nonEmpty(i.InvoiceNumber) && i.TotalAmount != nil
}

func nonEmpty(s *string) bool { return s != nil && *s != "" }

// RawJSONSchema mirrors RawInvoice (all string/nullable) and is injected into the
// prompt so the model is never asked to format numbers — postprocess does that.
const RawJSONSchema = `{
  "type": "object",
  "properties": {
    "vendor_name":    {"type": ["string","null"]},
    "vendor_tax_id":  {"type": ["string","null"], "description": "NPWP for Indonesian invoices"},
    "invoice_number": {"type": ["string","null"]},
    "invoice_date":   {"type": ["string","null"]},
    "due_date":       {"type": ["string","null"]},
    "currency":       {"type": ["string","null"], "description": "ISO 4217, e.g. IDR"},
    "line_items": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "description": {"type": "string"},
          "quantity":    {"type": ["string","null"]},
          "unit_price":  {"type": ["string","null"]},
          "amount":      {"type": ["string","null"]}
        },
        "required": ["description"]
      }
    },
    "subtotal":     {"type": ["string","null"]},
    "tax_amount":   {"type": ["string","null"], "description": "PPN where present"},
    "total_amount": {"type": ["string","null"]},
    "raw_notes":    {"type": ["string","null"]}
  }
}`
