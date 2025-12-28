package models

import (
	"time"

	"github.com/google/uuid"
)

type Invoice struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey"`
	InvoiceNumber string    `gorm:"uniqueIndex"`
	CustomerName  string    `gorm:"index"`
	CustomerEmail string
	Amount        float64 `gorm:"index"`
	Status        string  `gorm:"index"`
	DueDate       time.Time
	PaidAt        *time.Time
	CreatedAt     time.Time
}
