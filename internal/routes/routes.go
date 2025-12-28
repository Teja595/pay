package routes

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	handler "payment-reconciliation-backend/internal/handlers"
	"payment-reconciliation-backend/internal/repository"
	service "payment-reconciliation-backend/internal/services/reconciliation"
)

func RegisterRoutes(r *gin.Engine, db *gorm.DB) {
	invoiceRepo := repository.NewInvoiceRepository(db)
	transactionRepo := repository.NewBankTransactionRepository(db)

	reconService := service.NewReconciliationService(
		invoiceRepo,
		transactionRepo,
	)

	reconHandler := handler.NewReconciliationHandler(reconService)

	api := r.Group("/api")

	// Health check
	api.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Reconciliation batch routes
	recon := api.Group("/reconciliation")
	recon.POST("/upload", reconHandler.Upload)
	recon.GET("/:batchId", reconHandler.GetBatchProgress)
	recon.GET("/:batchId/transactions", reconHandler.ListTransactions)
	recon.POST("/:batchId/bulk-confirm", reconHandler.BulkConfirmAutoMatched)
	recon.GET("/matches", reconHandler.GetMatches) // optional placeholder
	recon.POST("/invoice", reconHandler.CreateInvoice)

	// Transaction-level routes
	tx := api.Group("/transactions")
	tx.POST("/:id/confirm", reconHandler.ConfirmTransaction)
	tx.POST("/:id/reject", reconHandler.RejectTransaction)
	tx.POST("/:id/match", reconHandler.ManualMatchTransaction)
	tx.POST("/:id/external", reconHandler.MarkTransactionExternal)

	// Invoice routes
	invoices := api.Group("/invoices")
	{
		invoices.POST("/upload", reconHandler.UploadInvoices)
	}
}
