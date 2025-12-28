package models

import (
	"time"

	"github.com/google/uuid"
)

type MatchAuditLog struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	TransactionID   uuid.UUID `gorm:"index"`
	Action          string
	PreviousInvoice *uuid.UUID
	NewInvoice      *uuid.UUID
	PerformedBy     string
	Reason          string
	CreatedAt       time.Time
}
