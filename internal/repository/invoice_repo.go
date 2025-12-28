package repository

import (
	"payment-reconciliation-backend/internal/models"
	"strings"
	"time"

	"gorm.io/gorm"
)

type InvoiceRepository struct {
	db *gorm.DB
}

func NewInvoiceRepository(db *gorm.DB) *InvoiceRepository {
	return &InvoiceRepository{db: db}
}

// Expose DB if needed
func (r *InvoiceRepository) DB() *gorm.DB {
	return r.db
}

// FindByAmount returns all invoices with the exact amount (float64)
func (r *InvoiceRepository) FindByAmount(amount float64) ([]models.Invoice, error) {
	var invoices []models.Invoice
	err := r.db.Where("amount = ?", amount).Find(&invoices).Error
	return invoices, err
}

// FindByNameAndAmount performs a fuzzy search (simple LIKE) for now
func (r *InvoiceRepository) FindByNameAndAmount(name string, amount float64) ([]models.Invoice, error) {
	var invoices []models.Invoice
	likeName := "%" + strings.ToLower(name) + "%"
	err := r.db.Where("LOWER(customer_name) LIKE ? AND amount = ?", likeName, amount).Find(&invoices).Error
	return invoices, err
}

// GetByID fetch a single invoice by ID
func (r *InvoiceRepository) GetByID(id string) (*models.Invoice, error) {
	var invoice models.Invoice
	err := r.db.First(&invoice, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &invoice, nil
}

// SearchInvoices used for admin manual search with optional filters
func (r *InvoiceRepository) SearchInvoices(query string, amount float64, statuses []string) ([]models.Invoice, error) {
	var invoices []models.Invoice

	dbQuery := r.db.Model(&models.Invoice{})

	if query != "" {
		dbQuery = dbQuery.Where("LOWER(customer_name) LIKE ?", "%"+strings.ToLower(query)+"%")
	}
	if amount > 0 {
		dbQuery = dbQuery.Where("amount = ?", amount)
	}
	if len(statuses) > 0 {
		dbQuery = dbQuery.Where("status IN ?", statuses)
	}

	err := dbQuery.Find(&invoices).Error
	return invoices, err
}

// invoice_repository.go
func (r *InvoiceRepository) FindMatchingInvoice(
	amount float64,
	txDate time.Time,
) (*models.Invoice, error) {

	var invoice models.Invoice

	err := r.db.
		Where("amount = ?", amount).
		Where("ABS(EXTRACT(EPOCH FROM (due_date - ?))) <= 259200", txDate). // 3 days
		Where("status IN ?", []string{"sent", "overdue"}).
		First(&invoice).Error

	if err != nil {
		return nil, err
	}

	return &invoice, nil
}
