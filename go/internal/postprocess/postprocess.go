// Package postprocess coerces the lenient RawInvoice (strings) into the typed,
// validated canonical Invoice for an expense record: locale number parsing, datetime
// parsing, category assignment (a vendor-keyword rule overrides the model), and
// computing total_amount by SUMMING the line item prices. We never trust the model's
// arithmetic, formatting, or printed grand total (often the cash tendered).
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

// ISO layouts are tried first on the ORIGINAL string (they contain a literal "T").
var isoLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// dayfirst (ID convention) layouts, with time variants first so a printed time is
// kept. Tried against the lowercased, month-name-translated input. Go matches month
// names case-insensitively, so capitalized "January"/"Jan" in the layout is fine.
var dateLayouts = []string{
	"02-01-2006 15:04:05", "2-1-2006 15:04:05",
	"02/01/2006 15:04:05", "2/1/2006 15:04:05",
	"02.01.2006 15:04:05",
	"02-01-2006 15:04", "02/01/2006 15:04",
	"02-01-06 15:04:05", "2-1-06 15:04:05",
	"02 January 2006 15:04:05", "2 January 2006 15:04",
	"02 Jan 2006 15:04:05",
	// date only -> midnight
	"2006/01/02",
	"02/01/2006", "2/1/2006",
	"02-01-2006", "2-1-2006",
	"02.01.2006", "2.1.2006",
	"02-01-06", "2-1-06",
	"02 January 2006", "2 January 2006",
	"02 Jan 2006", "2 Jan 2006",
	"January 2, 2006", "Jan 2, 2006",
}

// ParseDateTime parses a transaction date (and time when present). Day-first, tolerant
// of Indonesian month names; keeps the time when shown, midnight otherwise. Returns
// ok=false if no layout matches.
func ParseDateTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range isoLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
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

// ParseCategory canonicalizes a category to the known vocabulary; off-list -> "other",
// empty -> "" (the caller then falls back / leaves it null).
func ParseCategory(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	for _, c := range schema.Categories {
		if s == c {
			return s
		}
	}
	return "other"
}

// Vendor-keyword -> category. A deterministic rule is far more reliable than a small
// model for known Indonesian chains, so a match here overrides the model. First match
// wins; extend the keyword lists for your own vendors.
var vendorCategoryRules = []struct {
	category string
	keywords []string
}{
	{"medical", []string{"apotek", "apotik", "pharma", "farma", "klinik", "clinic", "hospital",
		"rumah sakit", "dental", "dokter", "medika", "optik"}},
	{"groceries", []string{"alfamart", "alfaria", "alfamidi", "indomaret", "indomarco",
		"supermarket", "minimarket", "superindo", "hypermart", "transmart",
		"giant", "lawson", "circle k", "familymart", "grosir", "swalayan"}},
	{"dining", []string{"resto", "restaurant", "rumah makan", "warung", "warteg", "cafe", "kafe",
		"coffee", "kopi", "kedai", "bakery", "pizza", "kfc", "mcdonald", "burger", "bakso", "kitchen"}},
	{"transport", []string{"pertamina", "spbu", "gojek", "grab", "parkir", "parking",
		"transjakarta", "kereta", "krl", "bensin"}},
	{"utilities", []string{"pln", "telkom", "indihome", "pdam", "biznet", "listrik", "pulsa"}},
}

// CategorizeByVendor returns a category from the vendor name via keyword rules, or ""
// if no rule fires (the caller then falls back to the model's own category).
func CategorizeByVendor(vendor string) string {
	if vendor == "" {
		return ""
	}
	low := strings.ToLower(vendor)
	for _, rule := range vendorCategoryRules {
		for _, kw := range rule.keywords {
			if strings.Contains(low, kw) {
				return rule.category
			}
		}
	}
	return ""
}

// Normalize coerces a RawInvoice into a typed canonical Invoice: it parses the
// datetime and each line price, assigns the category (vendor rule overriding the
// model), defaults the currency when configured, and computes total_amount by summing
// the line item prices. defaultCurrency "" leaves an absent currency null.
func Normalize(raw schema.RawInvoice, defaultCurrency string) schema.Invoice {
	inv := schema.Invoice{
		VendorName:          raw.VendorName,
		Currency:            raw.Currency,
		TransactionDatetime: parseDateTimePtr(raw.TransactionDatetime),
	}

	// Currency: default when the receipt printed none (opt-in; off by default).
	if defaultCurrency != "" && (inv.Currency == nil || strings.TrimSpace(*inv.Currency) == "") {
		c := defaultCurrency
		inv.Currency = &c
	}

	// Category: the vendor-keyword rule overrides the model's guess when it fires.
	if cat := categoryFor(raw); cat != "" {
		inv.Category = &cat
	}

	// Line items: parse each price, then compute the total ourselves.
	sum := decimal.Zero
	hasItems := false
	for _, ri := range raw.LineItems {
		amt := parseDecPtr(ri.Amount)
		inv.LineItems = append(inv.LineItems, schema.LineItem{Description: ri.Description, Amount: amt})
		if amt != nil {
			sum = sum.Add(*amt)
			hasItems = true
		}
	}
	if hasItems {
		inv.TotalAmount = &sum
	}
	return inv
}

func categoryFor(raw schema.RawInvoice) string {
	if raw.VendorName != nil {
		if c := CategorizeByVendor(*raw.VendorName); c != "" {
			return c
		}
	}
	if raw.Category != nil {
		if c := ParseCategory(*raw.Category); c != "" {
			return c
		}
	}
	return ""
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

func parseDateTimePtr(s *string) *time.Time {
	if s == nil {
		return nil
	}
	if t, ok := ParseDateTime(*s); ok {
		return &t
	}
	return nil
}
