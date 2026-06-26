// Package scoring computes field-level accuracy of a predicted Invoice against
// ground truth (the benchmark harness core).
package scoring

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"invoice-extractor/internal/schema"
)

// ScalarFields is the fixed display/scoring order for per-field accuracy.
var ScalarFields = []string{
	"vendor_name", "transaction_datetime", "currency", "category", "total_amount",
}

type InvoiceScore struct {
	FieldsTotal       int
	FieldsCorrect     int
	Accuracy          float64
	PerField          map[string]bool
	LineItemPrecision float64
	LineItemRecall    float64
	LineItemF1        float64
}

func Score(pred, gold schema.Invoice) InvoiceScore {
	pf := map[string]bool{
		"vendor_name":          eqStr(pred.VendorName, gold.VendorName),
		"transaction_datetime": eqDate(pred.TransactionDatetime, gold.TransactionDatetime),
		"currency":             eqStr(pred.Currency, gold.Currency),
		"category":             eqStr(pred.Category, gold.Category),
		"total_amount":         eqDec(pred.TotalAmount, gold.TotalAmount),
	}
	correct := 0
	for _, v := range pf {
		if v {
			correct++
		}
	}
	total := len(pf)
	p, r, f1 := scoreLineItems(pred.LineItems, gold.LineItems)
	return InvoiceScore{
		FieldsTotal:       total,
		FieldsCorrect:     correct,
		Accuracy:          float64(correct) / float64(total),
		PerField:          pf,
		LineItemPrecision: p,
		LineItemRecall:    r,
		LineItemF1:        f1,
	}
}

func norm(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

func eqStr(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return norm(*a) == norm(*b)
}

func eqDate(a, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Format("2006-01-02") == b.Format("2006-01-02")
}

func eqDec(a, b *decimal.Decimal) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

func lineEq(p, g schema.LineItem) bool {
	if norm(p.Description) != norm(g.Description) {
		return false
	}
	return eqDec(p.Amount, g.Amount)
}

func scoreLineItems(pred, gold []schema.LineItem) (precision, recall, f1 float64) {
	if len(pred) == 0 && len(gold) == 0 {
		return 1, 1, 1
	}
	used := make([]bool, len(pred))
	matched := 0
	for _, g := range gold {
		for i, p := range pred {
			if used[i] {
				continue
			}
			if lineEq(p, g) {
				used[i] = true
				matched++
				break
			}
		}
	}
	if len(pred) > 0 {
		precision = float64(matched) / float64(len(pred))
	}
	if len(gold) > 0 {
		recall = float64(matched) / float64(len(gold))
	}
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}
	return precision, recall, f1
}
