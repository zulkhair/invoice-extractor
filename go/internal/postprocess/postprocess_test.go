package postprocess

import (
	"testing"

	"github.com/shopspring/decimal"

	"invoice-extractor/internal/schema"
)

func sp(s string) *string { return &s }

func TestParseDecimalLocaleFormats(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"1.250.000,00", "1250000.00"}, // ID/EU
		{"1,250,000.00", "1250000.00"}, // US
		{"1250000", "1250000"},
		{"1.250.000", "1250000"}, // EU thousands, no decimals
		{"Rp 1.250.000", "1250000"},
		{"$1,234.56", "1234.56"},
		{"12,50", "12.50"},
		{"12.50", "12.50"},
		{"-50,00", "-50.00"},
		{"  7 ", "7"},
	}
	for _, c := range cases {
		got, ok := ParseDecimal(c.in)
		if !ok {
			t.Errorf("ParseDecimal(%q): ok=false, want %s", c.in, c.want)
			continue
		}
		if !got.Equal(decimal.RequireFromString(c.want)) {
			t.Errorf("ParseDecimal(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestParseDecimalUnparseable(t *testing.T) {
	for _, in := range []string{"", "   ", "N/A", "-", "abc"} {
		if _, ok := ParseDecimal(in); ok {
			t.Errorf("ParseDecimal(%q): ok=true, want false", in)
		}
	}
}

func TestParseDateTimeFormats(t *testing.T) {
	cases := []struct {
		in                 string
		y, mo, d, h, mi, s int
	}{
		{"19-05-2017 06:51:42", 2017, 5, 19, 6, 51, 42}, // dayfirst + time
		{"21/06/2026", 2026, 6, 21, 0, 0, 0},            // date only -> midnight
		{"2026-06-21T14:30:00", 2026, 6, 21, 14, 30, 0}, // ISO
		{"16-11-15 16:04:52", 2015, 11, 16, 16, 4, 52},  // 2-digit year + time
		{"21 Juni 2026 14:30", 2026, 6, 21, 14, 30, 0},  // Indonesian month + time
		{"21 Agustus 2025", 2025, 8, 21, 0, 0, 0},
	}
	for _, c := range cases {
		got, ok := ParseDateTime(c.in)
		if !ok {
			t.Errorf("ParseDateTime(%q): ok=false", c.in)
			continue
		}
		if got.Year() != c.y || int(got.Month()) != c.mo || got.Day() != c.d ||
			got.Hour() != c.h || got.Minute() != c.mi || got.Second() != c.s {
			t.Errorf("ParseDateTime(%q) = %v, want %d-%02d-%02d %02d:%02d:%02d",
				c.in, got, c.y, c.mo, c.d, c.h, c.mi, c.s)
		}
	}
}

func TestParseDateTimeUnparseable(t *testing.T) {
	for _, in := range []string{"", "not a date", "abc"} {
		if _, ok := ParseDateTime(in); ok {
			t.Errorf("ParseDateTime(%q): ok=true, want false", in)
		}
	}
}

func TestParseCategory(t *testing.T) {
	cases := []struct{ in, want string }{
		{"groceries", "groceries"},
		{"Groceries", "groceries"}, // case-normalized
		{"  MEDICAL ", "medical"},  // trimmed
		{"restaurant", "other"},    // off-list -> other
		{"", ""},
	}
	for _, c := range cases {
		if got := ParseCategory(c.in); got != c.want {
			t.Errorf("ParseCategory(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCategorizeByVendor(t *testing.T) {
	cases := []struct{ vendor, want string }{
		{"ALFAMART STA.KARET", "groceries"},
		{"PT INDOMARCO PRISMATAMA", "groceries"}, // legal-entity name matches
		{"Indomaret Cidatar", "groceries"},
		{"Lilac Beauty and Dental Clinic", "medical"},
		{"Warung Pasta @ Kemang", "dining"},
		{"SPBU Pertamina 34.567", "transport"},
		{"Toko Buku Gramedia", ""}, // no rule -> "" (fall back to model)
		{"", ""},
	}
	for _, c := range cases {
		if got := CategorizeByVendor(c.vendor); got != c.want {
			t.Errorf("CategorizeByVendor(%q) = %q, want %q", c.vendor, got, c.want)
		}
	}
}

func TestNormalizeComputesTotalCategoryDatetime(t *testing.T) {
	raw := schema.RawInvoice{
		VendorName:          sp("Warung Pasta @ Kemang"),
		TransactionDatetime: sp("16-11-15 16:04:52"),
		Currency:            sp("IDR"),
		LineItems: []schema.RawLineItem{
			{Description: "Cheezy Freezy M", Amount: sp("25.000")},
			{Description: "Lemon Tea", Amount: sp("11.000")},
		},
	}
	inv := Normalize(raw, "")
	if inv.TotalAmount == nil || !inv.TotalAmount.Equal(decimal.RequireFromString("36000")) {
		t.Fatalf("total = %v, want 36000 (computed sum)", inv.TotalAmount)
	}
	if inv.Category == nil || *inv.Category != "dining" { // vendor rule: "warung"
		t.Fatalf("category = %v, want dining", inv.Category)
	}
	if inv.TransactionDatetime == nil || inv.TransactionDatetime.Hour() != 16 {
		t.Fatalf("datetime = %v, want 16:04:52", inv.TransactionDatetime)
	}
	if len(inv.LineItems) != 2 || inv.LineItems[0].Amount == nil ||
		!inv.LineItems[0].Amount.Equal(decimal.RequireFromString("25000")) {
		t.Fatalf("line items = %v", inv.LineItems)
	}
}

func TestNormalizeTotalSummedNotFromModel(t *testing.T) {
	// The model's own total (often the cash tendered) is ignored; we sum the items.
	raw := schema.RawInvoice{
		LineItems:   []schema.RawLineItem{{Description: "A", Amount: sp("4.500")}},
		TotalAmount: sp("5.000"),
	}
	inv := Normalize(raw, "")
	if inv.TotalAmount == nil || !inv.TotalAmount.Equal(decimal.RequireFromString("4500")) {
		t.Fatalf("total = %v, want 4500", inv.TotalAmount)
	}
}

func TestNormalizeNoItemsTotalNil(t *testing.T) {
	inv := Normalize(schema.RawInvoice{VendorName: sp("X")}, "")
	if inv.TotalAmount != nil {
		t.Errorf("total = %v, want nil", inv.TotalAmount)
	}
}

func TestNormalizeVendorRuleOverridesModelCategory(t *testing.T) {
	inv := Normalize(schema.RawInvoice{VendorName: sp("Indomaret Cidatar"), Category: sp("shopping")}, "")
	if inv.Category == nil || *inv.Category != "groceries" {
		t.Errorf("category = %v, want groceries (vendor rule)", inv.Category)
	}
}

func TestNormalizeModelCategoryKeptWhenNoRule(t *testing.T) {
	inv := Normalize(schema.RawInvoice{VendorName: sp("Toko Buku Gramedia"), Category: sp("shopping")}, "")
	if inv.Category == nil || *inv.Category != "shopping" {
		t.Errorf("category = %v, want shopping", inv.Category)
	}
}

func TestNormalizeNullsUnparseable(t *testing.T) {
	inv := Normalize(schema.RawInvoice{
		TransactionDatetime: sp(""),
		LineItems:           []schema.RawLineItem{{Description: "X", Amount: sp("N/A")}},
	}, "")
	if inv.TransactionDatetime != nil {
		t.Errorf("datetime = %v, want nil", inv.TransactionDatetime)
	}
	if inv.TotalAmount != nil {
		t.Errorf("total = %v, want nil (no parseable amounts)", inv.TotalAmount)
	}
}

func TestNormalizeCurrencyDefault(t *testing.T) {
	if inv := Normalize(schema.RawInvoice{}, "IDR"); inv.Currency == nil || *inv.Currency != "IDR" {
		t.Errorf("absent currency = %v, want IDR", inv.Currency)
	}
	if inv := Normalize(schema.RawInvoice{Currency: sp("  ")}, "IDR"); inv.Currency == nil || *inv.Currency != "IDR" {
		t.Errorf("blank currency = %v, want IDR", inv.Currency)
	}
	if inv := Normalize(schema.RawInvoice{Currency: sp("USD")}, "IDR"); inv.Currency == nil || *inv.Currency != "USD" {
		t.Errorf("present currency = %v, want USD (not overridden)", inv.Currency)
	}
	if inv := Normalize(schema.RawInvoice{}, ""); inv.Currency != nil {
		t.Errorf("currency = %v, want nil (default off)", inv.Currency)
	}
}
