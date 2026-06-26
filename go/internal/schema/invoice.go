// Package schema defines the two-struct wire/canonical pattern for an expense record.
//
// RawInvoice is what the model emits: every numeric/date field is a *string so a
// malformed value (e.g. "1.250.000,00" or "21 Juni 2026") never breaks the whole
// unmarshal. postprocess coerces RawInvoice -> Invoice (typed, validated): it parses
// the datetime, parses each line item price, computes total_amount by SUMMING the line
// prices (the model's printed total is ignored), and assigns the category.
package schema

import (
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/shopspring/decimal"
)

// Categories is the fixed spending-category vocabulary. Anything off-list becomes
// "other"; a vendor-keyword rule (postprocess) overrides the model for known chains.
var Categories = []string{
	"groceries", "dining", "medical", "transport",
	"utilities", "shopping", "entertainment", "other",
}

// --- wire (lenient) ---

type RawLineItem struct {
	Description string  `json:"description"`
	Amount      *string `json:"amount"`
}

type RawInvoice struct {
	VendorName          *string       `json:"vendor_name"`
	TransactionDatetime *string       `json:"transaction_datetime"`
	Currency            *string       `json:"currency"` // ISO 4217, e.g. IDR
	Category            *string       `json:"category"`
	LineItems           []RawLineItem `json:"line_items"`
	TotalAmount         *string       `json:"total_amount"` // ignored; total is computed downstream
}

// --- canonical (typed, validated) ---

type LineItem struct {
	Description string           `json:"description"`
	Amount      *decimal.Decimal `json:"amount"`
}

type Invoice struct {
	VendorName          *string          `json:"vendor_name"`
	TransactionDatetime *time.Time       `json:"transaction_datetime"`
	Currency            *string          `json:"currency" validate:"omitempty,len=3"`
	Category            *string          `json:"category"`
	LineItems           []LineItem       `json:"line_items"`
	TotalAmount         *decimal.Decimal `json:"total_amount"` // computed = sum of line prices
}

var validate = validator.New()

// Validate runs struct validation on the canonical invoice.
func (i *Invoice) Validate() error { return validate.Struct(i) }

// HasRequiredish reports whether vendor and a (computed) total are both present.
// The text path falls back to vision when this is false.
func (i *Invoice) HasRequiredish() bool {
	return nonEmpty(i.VendorName) && i.TotalAmount != nil
}

func nonEmpty(s *string) bool { return s != nil && *s != "" }

// RawJSONSchema mirrors RawInvoice (all string/nullable) and is injected into the
// prompt so the model is never asked to format numbers — postprocess does that, and
// also computes total_amount, so the model is told to leave it null.
const RawJSONSchema = `{
  "type": "object",
  "properties": {
    "vendor_name":          {"type": ["string","null"], "description": "the store/business name where money was spent"},
    "transaction_datetime": {"type": ["string","null"], "description": "purchase date AND time exactly as printed"},
    "currency":             {"type": ["string","null"], "description": "ISO 4217, e.g. IDR"},
    "category":             {"type": ["string","null"], "description": "one of: groceries, dining, medical, transport, utilities, shopping, entertainment, other"},
    "line_items": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "description": {"type": "string"},
          "amount":      {"type": ["string","null"], "description": "the line price as printed, e.g. \"25.000\""}
        },
        "required": ["description"]
      }
    },
    "total_amount": {"type": ["string","null"], "description": "leave null — computed downstream by summing line prices"}
  }
}`
