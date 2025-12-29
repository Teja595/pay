package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type BankTransaction struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey"`
	UploadBatchID    uuid.UUID `gorm:"index"`
	TransactionDate  time.Time `gorm:"column:transaction_date"`
	Description      string
	Amount           float64 `gorm:"index"`
	ReferenceNumber  string
	Status           string `gorm:"index"`
	MatchedInvoiceID *uuid.UUID
	ConfidenceScore  float64
	MatchDetails     datatypes.JSON
	CreatedAt        time.Time
}
