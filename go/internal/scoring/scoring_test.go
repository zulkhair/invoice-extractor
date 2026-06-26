package scoring

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"invoice-extractor/internal/schema"
)

func sp(s string) *string { return &s }
func dp(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}
func tp(y, m, d int) *time.Time {
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	return &t
}

func gold() schema.Invoice {
	return schema.Invoice{
		VendorName:          sp("PT Buah Segar"),
		TransactionDatetime: tp(2026, 6, 21),
		Currency:            sp("IDR"),
		Category:            sp("groceries"),
		TotalAmount:         dp("277500"),
		LineItems: []schema.LineItem{
			{Description: "Mangga Harum Manis", Amount: dp("250000")},
			{Description: "Jeruk", Amount: dp("27500")},
		},
	}
}

func TestPerfectMatch(t *testing.T) {
	s := Score(gold(), gold())
	if s.FieldsCorrect != s.FieldsTotal || s.Accuracy != 1.0 {
		t.Errorf("accuracy = %v (%d/%d), want 1.0", s.Accuracy, s.FieldsCorrect, s.FieldsTotal)
	}
	if s.LineItemRecall != 1.0 || s.LineItemPrecision != 1.0 {
		t.Errorf("line items p=%v r=%v, want 1.0", s.LineItemPrecision, s.LineItemRecall)
	}
}

func TestWrongScalarCountsAgainst(t *testing.T) {
	pred := gold()
	pred.TotalAmount = dp("999")
	s := Score(pred, gold())
	if s.FieldsCorrect != s.FieldsTotal-1 {
		t.Errorf("correct = %d, want %d", s.FieldsCorrect, s.FieldsTotal-1)
	}
	if s.PerField["total_amount"] {
		t.Error("total_amount should be marked wrong")
	}
}

func TestStringMatchNormalizes(t *testing.T) {
	pred := gold()
	pred.VendorName = sp("  pt  BUAH segar ")
	s := Score(pred, gold())
	if !s.PerField["vendor_name"] {
		t.Error("vendor_name should match after normalization")
	}
}

func TestMissingLineItemLowersRecallNotPrecision(t *testing.T) {
	pred := gold()
	pred.LineItems = []schema.LineItem{{Description: "Mangga Harum Manis", Amount: dp("250000")}}
	s := Score(pred, gold())
	if s.LineItemRecall != 0.5 {
		t.Errorf("recall = %v, want 0.5", s.LineItemRecall)
	}
	if s.LineItemPrecision != 1.0 {
		t.Errorf("precision = %v, want 1.0", s.LineItemPrecision)
	}
}

func TestNullVsValue(t *testing.T) {
	g := schema.Invoice{VendorName: sp("X")}
	pred := schema.Invoice{}
	s := Score(pred, g)
	if s.PerField["vendor_name"] {
		t.Error("nil vs value should be wrong")
	}
	if !s.PerField["currency"] {
		t.Error("nil vs nil should be correct")
	}
}
