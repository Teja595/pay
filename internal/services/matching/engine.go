package matching

import (
	"math"
	"payment-reconciliation-backend/internal/models"
	"payment-reconciliation-backend/internal/services/reconciliation"
	"strings"
	"time"
)

func MatchTransaction(s *reconciliation.ReconciliationService, tx *models.BankTransaction) (*models.BankTransaction, error) {
	// 1. Find invoices with exact amount
	invoices, err := s.InvoiceRepo().FindByAmount(tx.Amount)
	if err != nil {
		return nil, err
	}

	// skip already-paid invoices
	var validInvoices []models.Invoice
	for _, inv := range invoices {
		if inv.Status != "paid" {
			validInvoices = append(validInvoices, inv)
		}
	}
	invoices = validInvoices

	if len(invoices) == 0 {
		tx.Status = "unmatched"
		s.DB().Save(tx)
		return tx, nil
	}

	// 2. Compute name similarity scores
	type candidate struct {
		invoice *models.Invoice
		score   float64
	}
	var candidates []candidate
	for _, inv := range invoices {
		score := computeNameSimilarity(tx.Description, inv.CustomerName)
		candidates = append(candidates, candidate{invoice: &inv, score: score})
	}

	// 3. Adjust for multiple invoices
	if len(candidates) > 1 {
		for i := range candidates {
			candidates[i].score *= 0.8
		}
	}

	// 4. Date proximity
	for i := range candidates {
		candidates[i].score += dateProximityScore(tx.TransactionDate, candidates[i].invoice.DueDate)
		if candidates[i].score > 100 {
			candidates[i].score = 100
		}
	}

	// 5. Pick best candidate
	best := candidates[0]
	for _, c := range candidates {
		if c.score > best.score {
			best = c
		}
	}

	// 6. Categorize
	switch {
	case best.score >= 95:
		tx.Status = "auto_matched"
	case best.score >= 60:
		tx.Status = "needs_review"
	default:
		tx.Status = "unmatched"
	}

	tx.MatchedInvoiceID = &best.invoice.ID
	tx.ConfidenceScore = best.score

	// optional: add match details
	// tx.MatchDetails = ...

	s.DB().Save(tx)
	return tx, nil
}

// Use Levenshtein or Jaro-Winkler (implement simple version or use a library)
func computeNameSimilarity(bankDesc, invoiceName string) float64 {
	b := normalizeName(bankDesc)
	i := normalizeName(invoiceName)
	// placeholder: simple ratio of common words
	wordsB := strings.Fields(b)
	wordsI := strings.Fields(i)
	matches := 0
	for _, w1 := range wordsB {
		for _, w2 := range wordsI {
			if w1 == w2 {
				matches++
			}
		}
	}
	return float64(matches) / math.Max(float64(len(wordsI)), 1) * 100
}

func normalizeName(name string) string {
	n := strings.ToUpper(name)
	n = strings.ReplaceAll(n, ".", "")
	n = strings.ReplaceAll(n, ",", "")
	return n
}

func dateProximityScore(txDate, dueDate time.Time) float64 {
	daysDiff := txDate.Sub(dueDate).Hours() / 24
	if daysDiff < 0 {
		return 5 // bonus if before due date
	}
	if daysDiff > 30 {
		return -10
	}
	return 0
}
