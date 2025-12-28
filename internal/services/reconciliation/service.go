package reconciliation

import (
	"encoding/json"
	"math"
	"strings"
	"time"

	"payment-reconciliation-backend/internal/models"
	"payment-reconciliation-backend/internal/repository"

	"sync"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ReconciliationService struct {
	invoiceRepo     *repository.InvoiceRepository
	transactionRepo *repository.BankTransactionRepository
	db              *gorm.DB
	progressCache   sync.Map // batchID -> *Progress
	statsCache      sync.Map // batchID -> *BatchStats
}

// 2. Compute name similarity scores
type candidate struct {
	invoice *models.Invoice
	score   float64
}
type Progress struct {
	ProcessedCount int
	Total          int
	Status         string
}

func NewReconciliationService(
	invoiceRepo *repository.InvoiceRepository,
	transactionRepo *repository.BankTransactionRepository,
) *ReconciliationService {
	return &ReconciliationService{
		invoiceRepo:     invoiceRepo,
		transactionRepo: transactionRepo,
		db:              invoiceRepo.DB(), // assuming repository exposes DB connection
	}
}

// CreateBatch creates a new ReconciliationBatch in DB
func (s *ReconciliationService) CreateBatch(filename string) *models.ReconciliationBatch {
	batch := &models.ReconciliationBatch{
		ID:        uuid.New(),
		Filename:  filename,
		Status:    "processing",
		StartedAt: time.Now(),
		CreatedAt: time.Now(),
	}

	s.db.Create(batch)
	return batch
}
func (s *ReconciliationService) MatchTransaction(tx *models.BankTransaction) (*models.BankTransaction, error) {
	// 1. Find invoices with exact amount
	invoices, err := s.invoiceRepo.FindByAmount(tx.Amount)
	if err != nil {
		return nil, err
	}

	if len(invoices) == 0 {
		tx.Status = "unmatched"
		s.db.Save(tx)
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

	// 3. Consider multiple invoices with same amount
	if len(candidates) > 1 {
		for i := range candidates {
			candidates[i].score *= 0.8 // reduce confidence due to ambiguity
		}
	}

	// 4. Adjust by date proximity
	for i := range candidates {
		candidates[i].score += dateProximityScore(tx.TransactionDate, candidates[i].invoice.DueDate)
		if candidates[i].score > 100 {
			candidates[i].score = 100
		}
	}

	// 5. Pick the best candidate
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
	// 7. Save match info
	tx.MatchedInvoiceID = &best.invoice.ID
	tx.ConfidenceScore = best.score

	details := map[string]interface{}{
		"amount_match":     true,
		"invoice_id":       best.invoice.ID.String(),
		"invoice_name":     best.invoice.CustomerName,
		"transaction_desc": tx.Description,
		"name_similarity":  best.score,
		"decision":         tx.Status,
		"candidate_count":  len(candidates),
	}

	detailsJSON, _ := json.Marshal(details)
	tx.MatchDetails = detailsJSON
	s.db.Save(tx)
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

// CreateTransaction inserts a single BankTransaction row

func (s *ReconciliationService) CreateTransaction(batchID uuid.UUID, description string, amount float64, reference string, date time.Time) *models.BankTransaction {
	tx := &models.BankTransaction{
		ID:              uuid.New(), // generate UUID
		UploadBatchID:   batchID,    // assign as uuid.UUID
		Description:     description,
		Amount:          amount,
		ReferenceNumber: reference,
		TransactionDate: date,
		Status:          "pending",
		CreatedAt:       time.Now(),
	}

	s.db.Create(tx)
	return tx
}
func (s *ReconciliationService) GetBatch(batchID uuid.UUID) (*models.ReconciliationBatch, error) {
	var batch models.ReconciliationBatch
	if err := s.db.First(&batch, "id = ?", batchID).Error; err != nil {
		return nil, err
	}
	return &batch, nil
}
func (s *ReconciliationService) ConfirmTransaction(txID uuid.UUID) (*models.BankTransaction, error) {
	var tx models.BankTransaction
	if err := s.db.First(&tx, "id = ?", txID).Error; err != nil {
		return nil, err
	}
	tx.Status = "confirmed"
	tx.ConfidenceScore = 100
	s.db.Save(&tx)
	return &tx, nil
}
func (s *ReconciliationService) CreateInvoice(invoiceNumber, name, email string, amount float64, status string, dueDate time.Time) *models.Invoice {
	inv := &models.Invoice{
		ID:            uuid.New(),
		InvoiceNumber: invoiceNumber,
		CustomerName:  name,
		CustomerEmail: email,
		Amount:        amount,
		Status:        status,
		DueDate:       dueDate,
		CreatedAt:     time.Now(),
	}

	// Use `OnConflict` to ignore duplicates
	s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(inv)
	return inv
}

func (s *ReconciliationService) RejectTransaction(txID uuid.UUID) (*models.BankTransaction, error) {
	var tx models.BankTransaction
	if err := s.db.First(&tx, "id = ?", txID).Error; err != nil {
		return nil, err
	}
	tx.Status = "unmatched"
	s.db.Save(&tx)
	return &tx, nil
}

func (s *ReconciliationService) ManualMatchTransaction(txID, invoiceID uuid.UUID) (*models.BankTransaction, error) {
	var tx models.BankTransaction
	if err := s.db.First(&tx, "id = ?", txID).Error; err != nil {
		return nil, err
	}
	tx.Status = "confirmed"
	tx.MatchedInvoiceID = &invoiceID
	tx.ConfidenceScore = 100
	s.db.Save(&tx)
	return &tx, nil
}

func (s *ReconciliationService) MarkTransactionExternal(txID uuid.UUID) (*models.BankTransaction, error) {
	var tx models.BankTransaction
	if err := s.db.First(&tx, "id = ?", txID).Error; err != nil {
		return nil, err
	}
	tx.Status = "external"
	tx.MatchedInvoiceID = nil
	tx.ConfidenceScore = 0
	s.db.Save(&tx)
	return &tx, nil
}
func (s *ReconciliationService) BulkConfirmAutoMatched(batchID uuid.UUID) (int64, error) {
	// Efficient bulk update in DB
	result := s.db.Model(&models.BankTransaction{}).
		Where("upload_batch_id = ? AND status = ?", batchID, "auto_matched").
		Updates(map[string]interface{}{
			"status":           "confirmed",
			"confidence_score": 100,
		})

	return result.RowsAffected, result.Error
}
func (s *ReconciliationService) ListTransactions(
	batchID uuid.UUID,
	status string,
	cursor string,
	limit int,
) ([]models.BankTransaction, string, bool) {

	var txs []models.BankTransaction
	query := s.db.
		Where("upload_batch_id = ?", batchID).
		Order("id ASC").
		Limit(limit + 1)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if cursor != "" {
		query = query.Where("id > ?", cursor)
	}

	query.Find(&txs)

	hasMore := false
	var nextCursor string

	if len(txs) > limit {
		hasMore = true
		nextCursor = txs[limit-1].ID.String()
		txs = txs[:limit]
	}

	return txs, nextCursor, hasMore
}

type BatchStats struct {
	Total       int64   `json:"total"`
	TotalAmount float64 `json:"total_amount"`

	AutoMatchedCount int64   `json:"auto_matched_count"`
	AutoMatchedSum   float64 `json:"auto_matched_sum"`

	NeedsReviewCount int64   `json:"needs_review_count"`
	NeedsReviewSum   float64 `json:"needs_review_sum"`

	UnmatchedCount int64   `json:"unmatched_count"`
	UnmatchedSum   float64 `json:"unmatched_sum"`

	ConfirmedCount int64   `json:"confirmed_count"`
	ConfirmedSum   float64 `json:"confirmed_sum"`
}
type StatRow struct {
	Status string
	Count  int64
	Sum    float64
}

func (s *ReconciliationService) GetBatchStats(batchID uuid.UUID) (BatchStats, error) {
	var stats BatchStats
	var rows []StatRow

	err := s.db.Model(&models.BankTransaction{}).
		Where("upload_batch_id = ?", batchID).
		Select("status, COUNT(*) as count, COALESCE(SUM(amount),0) as sum").
		Group("status").
		Scan(&rows).Error
	if err != nil {
		return stats, err
	}

	// Total
	var totalCount int64
	var totalSum float64
	for _, r := range rows {
		totalCount += r.Count
		totalSum += r.Sum

		switch r.Status {
		case "auto_matched":
			stats.AutoMatchedCount = r.Count
			stats.AutoMatchedSum = r.Sum
		case "needs_review":
			stats.NeedsReviewCount = r.Count
			stats.NeedsReviewSum = r.Sum
		case "unmatched":
			stats.UnmatchedCount = r.Count
			stats.UnmatchedSum = r.Sum
		case "confirmed":
			stats.ConfirmedCount = r.Count
			stats.ConfirmedSum = r.Sum
		}
	}

	stats.Total = totalCount
	stats.TotalAmount = totalSum

	return stats, nil
}

func (s *ReconciliationService) UpdateBatchProgressCache(batchID uuid.UUID, count int) {
	val, _ := s.progressCache.LoadOrStore(batchID, &Progress{
		ProcessedCount: 0,
		Total:          0,
		Status:         "processing",
	})
	p := val.(*Progress)
	p.ProcessedCount = count
	s.progressCache.Store(batchID, p)
}

// Mark completed
func (s *ReconciliationService) MarkBatchCompletedCache(batchID uuid.UUID, total int) {
	val, _ := s.progressCache.Load(batchID)
	p := val.(*Progress)
	p.ProcessedCount = total
	p.Total = total
	p.Status = "completed"
	s.progressCache.Store(batchID, p)

	// Optionally persist final state to DB
	s.MarkBatchCompleted(batchID, total)
}

func (s *ReconciliationService) GetBatchStatsCache(batchID uuid.UUID) BatchStats {
	if val, ok := s.statsCache.Load(batchID); ok {
		return *val.(*BatchStats)
	}

	stats, err := s.GetBatchStats(batchID)
	if err != nil {
		return BatchStats{}
	}

	s.statsCache.Store(batchID, &stats)
	return stats
}

// UpdateBatchProgress updates the processed count in a batch
func (s *ReconciliationService) UpdateBatchProgress(batchID uuid.UUID, count int) {
	s.db.Model(&models.ReconciliationBatch{}).
		Where("id = ?", batchID).
		Update("processed_count", count)
}

// MarkBatchCompleted sets batch status to completed
func (s *ReconciliationService) MarkBatchCompleted(batchID uuid.UUID, total int) {
	s.db.Model(&models.ReconciliationBatch{}).
		Where("id = ?", batchID).
		Updates(map[string]interface{}{
			"status":             "completed",
			"processed_count":    total,
			"total_transactions": total, // <-- add this line
			"completed_at":       time.Now(),
		})
}

func (s *ReconciliationService) InvoiceRepo() *repository.InvoiceRepository {
	return s.invoiceRepo
}

func (s *ReconciliationService) TransactionRepo() *repository.BankTransactionRepository {
	return s.transactionRepo
}

func (s *ReconciliationService) DB() *gorm.DB {
	return s.db
}
