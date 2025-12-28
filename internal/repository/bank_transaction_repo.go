package repository

import "gorm.io/gorm"

type BankTransactionRepository struct {
	db *gorm.DB
}

func NewBankTransactionRepository(db *gorm.DB) *BankTransactionRepository {
	return &BankTransactionRepository{db: db}
}
