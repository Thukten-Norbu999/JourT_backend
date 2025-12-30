package models

import (
	"time"

	"gorm.io/gorm"
)

type BacktestRun struct {
	gorm.Model
	ID     uint `gorm:"primaryKey" json:"id"`
	UserID uint `gorm:"index" json:"-"`

	Strategy  string    `json:"strategy"`
	Symbol    string    `json:"symbol"`
	Timeframe string    `json:"timeframe"`
	FromDate  time.Time `json:"from"`
	ToDate    time.Time `json:"to"`

	SummaryJSON string    `json:"summary"`
	CreatedAt   time.Time `json:"createdAt"`
}
