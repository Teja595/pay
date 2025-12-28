package handler

import (
	"encoding/csv"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	service "payment-reconciliation-backend/internal/services/reconciliation"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	// matching "payment-reconciliation-backend/internal/services/matching"
)

// Run is a simple endpoint placeholder
func (h *ReconciliationHandler) Run(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "reconciliation started"})
}

// GetMatches is a placeholder for now
func (h *ReconciliationHandler) GetMatches(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"data": []string{}})
}

// reconciliation_handler.go
func (h *ReconciliationHandler) GetBatchProgress(c *gin.Context) {
	batchID := c.Param("batchId")
	batch, err := h.service.GetBatch(uuid.MustParse(batchID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"processed_count": batch.ProcessedCount,
		"total":           batch.TotalTransactions,
		"status":          batch.Status,
	})
}
func (h *ReconciliationHandler) ConfirmTransaction(c *gin.Context) {
	txID := c.Param("id")
	id, err := uuid.Parse(txID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid transaction ID"})
		return
	}

	tx, err := h.service.ConfirmTransaction(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "transaction confirmed", "transaction": tx})
}

func (h *ReconciliationHandler) RejectTransaction(c *gin.Context) {
	txID := c.Param("id")
	id, err := uuid.Parse(txID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid transaction ID"})
		return
	}

	tx, err := h.service.RejectTransaction(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "transaction rejected", "transaction": tx})
}

func (h *ReconciliationHandler) ManualMatchTransaction(c *gin.Context) {
	txID := c.Param("id")
	id, err := uuid.Parse(txID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid transaction ID"})
		return
	}

	var payload struct {
		InvoiceID string `json:"invoice_id"`
	}
	if err := c.BindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	invoiceUUID, err := uuid.Parse(payload.InvoiceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid invoice ID"})
		return
	}

	tx, err := h.service.ManualMatchTransaction(id, invoiceUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "transaction manually matched", "transaction": tx})
}
func (h *ReconciliationHandler) CreateInvoice(c *gin.Context) {
	var payload struct {
		InvoiceNumber string  `json:"invoice_number"` // optional
		CustomerName  string  `json:"customer_name"`
		CustomerEmail string  `json:"customer_email"`
		Amount        float64 `json:"amount"`
		Status        string  `json:"status"`
		DueDate       string  `json:"due_date"` // "dd-mm-yyyy"
	}

	if err := c.BindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	if payload.CustomerName == "" || payload.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid customer name or amount"})
		return
	}

	// Parse due date
	dueDate, err := time.Parse("02-01-2006", payload.DueDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid due date format, expected dd-mm-yyyy"})
		return
	}

	// Generate invoice number if missing
	invoiceNumber := payload.InvoiceNumber
	if invoiceNumber == "" {
		invoiceNumber = uuid.New().String()
	}

	// Create invoice
	invoice := h.service.CreateInvoice(invoiceNumber, payload.CustomerName, payload.CustomerEmail, payload.Amount, payload.Status, dueDate)

	c.JSON(http.StatusOK, gin.H{"message": "invoice created", "invoice": invoice})
}

func (h *ReconciliationHandler) MarkTransactionExternal(c *gin.Context) {
	txID := c.Param("id")
	id, err := uuid.Parse(txID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid transaction ID"})
		return
	}

	tx, err := h.service.MarkTransactionExternal(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "transaction marked as external", "transaction": tx})
}
func (h *ReconciliationHandler) BulkConfirmAutoMatched(c *gin.Context) {
	batchIDStr := c.Param("batchId")
	batchID, err := uuid.Parse(batchIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid batch ID"})
		return
	}

	count, err := h.service.BulkConfirmAutoMatched(batchID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":              "bulk confirm completed",
		"transactions_updated": count,
	})
}

func (h *ReconciliationHandler) ListTransactions(c *gin.Context) {
	batchIDStr := c.Param("batchId")
	batchID, err := uuid.Parse(batchIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid batch ID"})
		return
	}

	status := c.Query("status")
	cursor := c.Query("cursor")
	limit := 50

	items, nextCursor, hasMore := h.service.ListTransactions(batchID, status, cursor, limit)
	stats, _ := h.service.GetBatchStats(batchID) // ignore error for now

	c.JSON(http.StatusOK, gin.H{
		"items":       items,
		"next_cursor": nextCursor,
		"has_more":    hasMore,
		"stats":       stats,
	})
}

// Confirm is a placeholder for now
func (h *ReconciliationHandler) Confirm(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "confirmed"})
}
func (h *ReconciliationHandler) UploadInvoices(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		log.Println("ERROR: no file received")
		c.JSON(http.StatusBadRequest, gin.H{"error": "file required"})
		return
	}
	defer file.Close()

	log.Println("Received file:", header.Filename, "size:", header.Size)

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	sample, _ := file.Read(make([]byte, 1024)) // read first KB
	file.Seek(0, 0)                            // reset reader
	if strings.Contains(string(sample), ",") {
		reader.Comma = ','
	} else if strings.Contains(string(sample), "\t") {
		reader.Comma = '\t'
	}

	// Read header
	headerRow, err := reader.Read()
	if err != nil {
		log.Println("ERROR reading header:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot read CSV header"})
		return
	}
	log.Println("CSV Header:", headerRow)

	inserted := 0
	rowNum := 0

	for {
		record, err := reader.Read()
		rowNum++

		if err == io.EOF {
			log.Println("Reached end of file")
			break
		}
		if err != nil {
			log.Printf("ERROR reading row %d: %v\n", rowNum, err)
			continue
		}

		log.Printf("RAW row %d, len=%d: %q\n", rowNum, len(record), record)

		if len(record) < 9 {
			log.Printf("Skipping row %d, insufficient columns\n", rowNum)
			continue
		}

		invoiceNumber := strings.TrimSpace(record[1])
		if invoiceNumber == "" {
			invoiceNumber = uuid.New().String()
			log.Printf("Row %d: Invoice number empty, generated %s\n", rowNum, invoiceNumber)
		}

		customerName := strings.TrimSpace(record[2])
		customerEmail := strings.TrimSpace(record[3])
		amountStr := strings.TrimSpace(record[4])
		status := strings.TrimSpace(record[5])
		dueDateStr := strings.TrimSpace(record[6])

		amount, err := strconv.ParseFloat(amountStr, 64)
		if err != nil || amount <= 0 {
			log.Printf("Skipping row %d: invalid amount=%s\n", rowNum, amountStr)
			continue
		}
		if customerName == "" {
			log.Printf("Skipping row %d: customer name empty\n", rowNum)
			continue
		}

		var dueDate time.Time
		dueDateStr = strings.TrimSpace(record[6])
		dueDate, err = time.Parse("2006-01-02", dueDateStr)
		if err != nil {
			dueDate, err = time.Parse("02-01-2006", dueDateStr)
		}
		if err != nil {
			log.Printf("Skipping row %d: invalid due date=%s", rowNum, dueDateStr)
			continue
		}

		log.Printf(
			"Row %d: Invoice=%s, Customer=%s, Email=%s, Amount=%.2f, Status=%s, DueDate=%s\n",
			rowNum, invoiceNumber, customerName, customerEmail, amount, status, dueDate.Format("2006-01-02"),
		)

		h.service.CreateInvoice(invoiceNumber, customerName, customerEmail, amount, status, dueDate)
		inserted++
	}

	log.Println("Total invoices inserted:", inserted)

	c.JSON(http.StatusOK, gin.H{
		"file":          header.Filename,
		"invoicesAdded": inserted,
	})
}

// Upload handles CSV uploads, creates a batch, and processes in background
func (h *ReconciliationHandler) Upload(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file required"})
		return
	}
	defer file.Close()

	// Create batch in DB (service generates UUID)
	batch := h.service.CreateBatch(header.Filename)

	// Process CSV in background (pass batch.ID as uuid.UUID)
	go h.processCSV(batch.ID, file)

	c.JSON(http.StatusAccepted, gin.H{
		"batch_id": batch.ID.String(), // send string in JSON
		"status":   "processing",
	})
}
func (h *ReconciliationHandler) processCSV(batchID uuid.UUID, reader io.Reader) {
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1

	// Skip header
	_, _ = csvReader.Read()

	count := 0

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // skip malformed rows
		}

		// Skip completely blank rows
		if len(record) == 0 || strings.Join(record, "") == "" {
			continue
		}

		// Parse amount
		amount, err := strconv.ParseFloat(record[3], 64)
		if err != nil {
			continue // skip if amount invalid
		}

		// Parse date
		date, err := time.Parse("02-01-2006", record[1])
		if err != nil {
			continue // skip if date invalid
		}

		// Create transaction
		tx := h.service.CreateTransaction(batchID, record[2], amount, record[4], date)

		// *** Run automatic matching immediately ***
		h.service.MatchTransaction(tx)

		count++

		// Update progress every 100 rows
		if count%100 == 0 {
			h.service.UpdateBatchProgress(batchID, count)
		}
	}

	// Final batch completion
	h.service.MarkBatchCompleted(batchID, count)

	// Final batch completion
	h.service.MarkBatchCompleted(batchID, count)
}

type ReconciliationHandler struct {
	service *service.ReconciliationService
}

func NewReconciliationHandler(s *service.ReconciliationService) *ReconciliationHandler {
	return &ReconciliationHandler{service: s}
}
