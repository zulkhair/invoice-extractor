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

func TestParseDateFormats(t *testing.T) {
	cases := []struct {
		in              string
		y               int
		m               int
		d               int
	}{
		{"21/06/2026", 2026, 6, 21}, // dayfirst
		{"21-06-2026", 2026, 6, 21},
		{"2026-06-21", 2026, 6, 21},
		{"21 Jun 2026", 2026, 6, 21},
		{"21 Juni 2026", 2026, 6, 21},   // Indonesian
		{"21 Agustus 2025", 2025, 8, 21},
	}
	for _, c := range cases {
		got, ok := ParseDate(c.in)
		if !ok {
			t.Errorf("ParseDate(%q): ok=false", c.in)
			continue
		}
		if got.Year() != c.y || int(got.Month()) != c.m || got.Day() != c.d {
			t.Errorf("ParseDate(%q) = %v, want %d-%02d-%02d", c.in, got, c.y, c.m, c.d)
		}
	}
}

func TestParseDateUnparseable(t *testing.T) {
	for _, in := range []string{"", "not a date", "99/99/9999"} {
		if _, ok := ParseDate(in); ok {
			t.Errorf("ParseDate(%q): ok=true, want false", in)
		}
	}
}

func TestNormalizeProducesTypedInvoice(t *testing.T) {
	raw := schema.RawInvoice{
		VendorName:    sp("PT Buah Segar"),
		InvoiceNumber: sp("INV/2026/0042"),
		InvoiceDate:   sp("21/06/2026"),
		Currency:      sp("IDR"),
		LineItems: []schema.RawLineItem{
			{Description: "Mangga", Quantity: sp("10"), UnitPrice: sp("25.000"), Amount: sp("250.000")},
		},
		Subtotal:    sp("250.000"),
		TaxAmount:   sp("27.500"),
		TotalAmount: sp("277.500"),
	}
	inv := Normalize(raw)
	if inv.TotalAmount == nil || !inv.TotalAmount.Equal(decimal.RequireFromString("277500")) {
		t.Fatalf("total = %v, want 277500", inv.TotalAmount)
	}
	if inv.InvoiceDate == nil || inv.InvoiceDate.Day() != 21 {
		t.Fatalf("invoice_date = %v", inv.InvoiceDate)
	}
	if len(inv.LineItems) != 1 || inv.LineItems[0].Amount == nil ||
		!inv.LineItems[0].Amount.Equal(decimal.RequireFromString("250000")) {
		t.Fatalf("line item amount = %v", inv.LineItems)
	}
}

func TestNormalizeNullsUnparseable(t *testing.T) {
	raw := schema.RawInvoice{TotalAmount: sp("N/A"), InvoiceDate: sp("")}
	inv := Normalize(raw)
	if inv.TotalAmount != nil {
		t.Errorf("total = %v, want nil", inv.TotalAmount)
	}
	if inv.InvoiceDate != nil {
		t.Errorf("date = %v, want nil", inv.InvoiceDate)
	}
}

func TestReconcileConsistent(t *testing.T) {
	d := decimal.RequireFromString
	amt100, amt200 := d("100"), d("200")
	sub, tax, tot := d("300"), d("33"), d("333")
	inv := schema.Invoice{
		LineItems: []schema.LineItem{{Description: "A", Amount: &amt100}, {Description: "B", Amount: &amt200}},
		Subtotal:  &sub, TaxAmount: &tax, TotalAmount: &tot,
	}
	Reconcile(&inv)
	if !inv.Consistent {
		t.Error("expected Consistent=true")
	}
}

func TestReconcileFlagsMismatch(t *testing.T) {
	d := decimal.RequireFromString
	amt100, amt200 := d("100"), d("200")
	sub, tot := d("300"), d("999")
	inv := schema.Invoice{
		LineItems: []schema.LineItem{{Description: "A", Amount: &amt100}, {Description: "B", Amount: &amt200}},
		Subtotal:  &sub, TotalAmount: &tot,
	}
	Reconcile(&inv)
	if inv.Consistent {
		t.Error("expected Consistent=false on total mismatch")
	}
}

func TestReconcileMissingDataIsConsistent(t *testing.T) {
	d := decimal.RequireFromString
	tot := d("500")
	inv := schema.Invoice{TotalAmount: &tot}
	Reconcile(&inv)
	if !inv.Consistent {
		t.Error("missing data should not be flagged inconsistent")
	}
}
