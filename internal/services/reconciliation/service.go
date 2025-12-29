package reconciliation

import (
	"encoding/json"
	"log"
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
	// Invoice cache: amount -> list of invoices
	invoiceCache map[float64][]*models.Invoice
}

func (s *ReconciliationService) LoadInvoiceCache() error {
	invoices, err := s.invoiceRepo.GetAll()
	if err != nil {
		return err
	}

	cache := make(map[float64][]*models.Invoice)
	for i := range invoices {
		cache[invoices[i].Amount] = append(cache[invoices[i].Amount], &invoices[i])
	}

	s.invoiceCache = cache
	log.Println("Invoice cache loaded, total amounts:", len(cache))
	return nil
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

	// 1. Lookup invoices from cache instead of DB
	invoices := s.invoiceCache[tx.Amount]

	if len(invoices) == 0 {
		tx.Status = "unmatched"
		s.db.Save(tx)
		return tx, nil
	}

	type candidate struct {
		invoice        *models.Invoice
		nameScore      float64
		dateScore      float64
		ambiguityScore float64
		finalScore     float64
	}

	var candidates []candidate

	for _, inv := range invoices {
		nameScore := computeNameSimilarity(tx.Description, inv.CustomerName)
		dateScore := computeDateScore(tx.TransactionDate, inv.DueDate)

		candidates = append(candidates, candidate{
			invoice:   inv,
			nameScore: nameScore,
			dateScore: dateScore,
		})
	}

	// 2. Ambiguity penalty if multiple invoices
	ambiguityScore := 100.0
	if len(candidates) > 1 {
		ambiguityScore = 80.0
	}

	// 3. Final weighted confidence score
	for i := range candidates {
		candidates[i].ambiguityScore = ambiguityScore
		candidates[i].finalScore =
			0.6*candidates[i].nameScore +
				0.3*candidates[i].dateScore +
				0.1*candidates[i].ambiguityScore
	}

	// 4. Pick best candidate
	best := candidates[0]
	for _, c := range candidates {
		if c.finalScore > best.finalScore {
			best = c
		}
	}

	// 5. Categorize
	switch {
	case best.finalScore >= 90:
		tx.Status = "auto_matched"
	case best.finalScore >= 60:
		tx.Status = "needs_review"
	default:
		tx.Status = "unmatched"
	}

	tx.MatchedInvoiceID = &best.invoice.ID
	tx.ConfidenceScore = math.Min(best.finalScore, 100)

	// 6. Persist match details
	details := map[string]interface{}{
		"amount_match":     true,
		"invoice_id":       best.invoice.ID.String(),
		"invoice_name":     best.invoice.CustomerName,
		"transaction_desc": tx.Description,
		"name_score":       best.nameScore,
		"date_score":       best.dateScore,
		"ambiguity_score":  best.ambiguityScore,
		"final_score":      best.finalScore,
		"candidate_count":  len(candidates),
		"decision":         tx.Status,
	}

	detailsJSON, _ := json.Marshal(details)
	tx.MatchDetails = detailsJSON

	s.db.Save(tx)
	return tx, nil
}

// Use Levenshtein or Jaro-Winkler (implement simple version or use a library)
func computeNameSimilarity(bankDesc, invoiceName string) float64 {
	bTokens := strings.Fields(normalizeName(bankDesc))
	iTokens := strings.Fields(normalizeName(invoiceName))

	if len(iTokens) == 0 {
		return 0
	}

	totalScore := 0.0

	for _, invTok := range iTokens {
		best := 0.0
		for _, bankTok := range bTokens {
			dist := levenshtein(invTok, bankTok)
			maxLen := math.Max(float64(len(invTok)), float64(len(bankTok)))
			sim := 1 - float64(dist)/maxLen
			if sim > best {
				best = sim
			}
		}
		totalScore += best
	}

	return (totalScore / float64(len(iTokens))) * 100
}

func normalizeName(s string) string {
	s = strings.ToUpper(s)
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.TrimSpace(s)
	return s
}
func computeDateScore(txDate, dueDate time.Time) float64 {
	days := math.Abs(txDate.Sub(dueDate).Hours() / 24)

	switch {
	case days <= 3:
		return 100
	case days <= 7:
		return 80
	case days <= 15:
		return 60
	case days <= 30:
		return 40
	default:
		return 20
	}
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

func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	dp := make([][]int, len(a)+1)
	for i := range dp {
		dp[i] = make([]int, len(b)+1)
	}

	for i := 0; i <= len(a); i++ {
		dp[i][0] = i
	}
	for j := 0; j <= len(b); j++ {
		dp[0][j] = j
	}

	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			dp[i][j] = min(
				dp[i-1][j]+1,
				dp[i][j-1]+1,
				dp[i-1][j-1]+cost,
			)
		}
	}
	return dp[len(a)][len(b)]
}

func min(a, b, c int) int {
	if a < b && a < c {
		return a
	}
	if b < c {
		return b
	}
	return c
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
	// log.Println("batchhh ", batch)
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
	search string, // <-- new parameter
) ([]models.BankTransaction, string, bool) {

	var txs []models.BankTransaction
	query := s.db.
		Where("upload_batch_id = ?", batchID).
		Order("id ASC").
		Limit(limit + 1)

	// filter by status
	if status != "" && status != "all" {
		query = query.Where("status = ?", status)
	}

	// filter by cursor
	if cursor != "" {
		query = query.Where("id > ?", cursor)
	}

	// filter by search (description or amount)
	if search != "" {
		likeQuery := "%" + search + "%"
		query = query.Where(
			"description ILIKE ? OR CAST(amount AS TEXT) LIKE ?",
			likeQuery, likeQuery,
		)
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
func (s *ReconciliationService) UpdateBatchProgress(id uuid.UUID, count int) error {
	return s.db.Model(&models.ReconciliationBatch{}).
		Where("id = ?", id).
		Update("processed_count", count).
		Error
}

// MarkBatchCompleted sets batch status to completed
func (s *ReconciliationService) MarkBatchCompleted(batchID uuid.UUID, count int) error {
	return s.db.Model(&models.ReconciliationBatch{}).
		Where("id = ?", batchID).
		Updates(map[string]interface{}{
			"processed_count":    count,
			"total_transactions": count, // ✅ FIXED
			"status":             "completed",
			"completed_at":       time.Now(),
		}).Error
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
