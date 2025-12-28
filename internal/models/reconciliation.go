package models

import (
	"time"

	"github.com/google/uuid"
)

type ReconciliationBatch struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	Filename          string
	TotalTransactions int
	ProcessedCount    int
	AutoMatchedCount  int
	NeedsReviewCount  int
	UnmatchedCount    int
	Status            string
	StartedAt         time.Time
	CompletedAt       *time.Time
	CreatedAt         time.Time
}
