// Package postprocess coerces the lenient RawInvoice (strings) into the typed,
// validated canonical Invoice (spec Task 5): locale number parsing, date parsing,
// and total reconciliation. We never trust the model's arithmetic or formatting.
package postprocess

import (
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"invoice-extractor/internal/schema"
)

var (
	negRe   = regexp.MustCompile(`-\s*\d`)
	stripRe = regexp.MustCompile(`[^0-9.,]`)
	digitRe = regexp.MustCompile(`\d`)
)

// ParseDecimal parses a locale-formatted money value into a Decimal.
// Handles "1.250.000,00" (ID/EU), "1,250,000.00" (US), currency prefixes, and
// negatives. Returns ok=false for anything unparseable — never fabricates.
func ParseDecimal(s string) (decimal.Decimal, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return decimal.Zero, false
	}
	neg := negRe.MatchString(s) || (strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")"))
	core := stripRe.ReplaceAllString(s, "")
	if !digitRe.MatchString(core) {
		return decimal.Zero, false
	}
	core = stripSeparators(core)
	d, err := decimal.NewFromString(core)
	if err != nil {
		return decimal.Zero, false
	}
	if neg {
		d = d.Neg()
	}
	return d, true
}

func stripSeparators(core string) string {
	hasDot := strings.Contains(core, ".")
	hasComma := strings.Contains(core, ",")
	switch {
	case hasDot && hasComma:
		// rightmost separator is the decimal point; the other is grouping
		if strings.LastIndex(core, ",") > strings.LastIndex(core, ".") {
			return strings.ReplaceAll(strings.ReplaceAll(core, ".", ""), ",", ".")
		}
		return strings.ReplaceAll(core, ",", "")
	case hasComma:
		return resolveSingle(core, ",")
	case hasDot:
		return resolveSingle(core, ".")
	default:
		return core
	}
}

func resolveSingle(core, sep string) string {
	parts := strings.Split(core, sep)
	if len(parts) > 2 {
		return strings.ReplaceAll(core, sep, "") // grouping separator
	}
	left, right := parts[0], parts[1]
	if len(right) == 3 && len(left) >= 1 {
		return left + right // "1.250" -> 1250
	}
	return left + "." + right // "12,50" -> 12.50
}

var idMonths = map[string]string{
	"januari": "january", "februari": "february", "maret": "march",
	"april": "april", "mei": "may", "juni": "june", "juli": "july",
	"agustus": "august", "september": "september", "oktober": "october",
	"november": "november", "desember": "december",
}

// dayfirst layouts (ID convention) plus ISO and month-name forms. Go matches
// month/day names case-insensitively, so lowercased input parses fine.
var dateLayouts = []string{
	"2006-01-02", "2006/01/02",
	"02/01/2006", "2/1/2006",
	"02-01-2006", "2-1-2006",
	"02.01.2006", "2.1.2006",
	"02 January 2006", "2 January 2006",
	"02 Jan 2006", "2 Jan 2006",
	"January 2, 2006", "Jan 2, 2006",
}

// ParseDate parses a date string into a time.Time. Day-first, tolerant of
// Indonesian month names. Returns ok=false if no layout matches.
func ParseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	low := strings.ToLower(s)
	for id, en := range idMonths {
		if strings.Contains(low, id) {
			low = strings.ReplaceAll(low, id, en)
		}
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, low); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// Normalize coerces a RawInvoice into a typed canonical Invoice and reconciles totals.
func Normalize(raw schema.RawInvoice) schema.Invoice {
	inv := schema.Invoice{
		VendorName:    raw.VendorName,
		VendorTaxID:   raw.VendorTaxID,
		InvoiceNumber: raw.InvoiceNumber,
		Currency:      raw.Currency,
		RawNotes:      raw.RawNotes,
		InvoiceDate:   parseDatePtr(raw.InvoiceDate),
		DueDate:       parseDatePtr(raw.DueDate),
		Subtotal:      parseDecPtr(raw.Subtotal),
		TaxAmount:     parseDecPtr(raw.TaxAmount),
		TotalAmount:   parseDecPtr(raw.TotalAmount),
	}
	for _, ri := range raw.LineItems {
		inv.LineItems = append(inv.LineItems, schema.LineItem{
			Description: ri.Description,
			Quantity:    parseDecPtr(ri.Quantity),
			UnitPrice:   parseDecPtr(ri.UnitPrice),
			Amount:      parseDecPtr(ri.Amount),
		})
	}
	Reconcile(&inv)
	return inv
}

func parseDecPtr(s *string) *decimal.Decimal {
	if s == nil {
		return nil
	}
	if d, ok := ParseDecimal(*s); ok {
		return &d
	}
	return nil
}

func parseDatePtr(s *string) *time.Time {
	if s == nil {
		return nil
	}
	if t, ok := ParseDate(*s); ok {
		return &t
	}
	return nil
}

// Reconcile recomputes line-item sum vs subtotal vs total and sets Consistent.
// A check that can't run (missing data) does not make the invoice inconsistent.
func Reconcile(inv *schema.Invoice) {
	tol := decimal.RequireFromString("0.02")

	var itemsSum *decimal.Decimal
	sum := decimal.Zero
	hasItems := false
	for _, li := range inv.LineItems {
		if li.Amount != nil {
			sum = sum.Add(*li.Amount)
			hasItems = true
		}
	}
	if hasItems {
		itemsSum = &sum
	}

	subtotalOK := true
	if itemsSum != nil && inv.Subtotal != nil {
		subtotalOK = itemsSum.Sub(*inv.Subtotal).Abs().LessThanOrEqual(tol)
	}

	totalOK := true
	base := inv.Subtotal
	if base == nil {
		base = itemsSum
	}
	if inv.TotalAmount != nil && base != nil {
		expected := *base
		if inv.TaxAmount != nil {
			expected = expected.Add(*inv.TaxAmount)
		}
		totalOK = expected.Sub(*inv.TotalAmount).Abs().LessThanOrEqual(tol)
	}

	inv.Consistent = subtotalOK && totalOK
}
