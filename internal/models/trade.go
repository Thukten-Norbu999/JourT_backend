package models

import (
	"time"

	"gorm.io/gorm"
)

type Trade struct {
	gorm.Model
	ID     uint `gorm:"primaryKey" json:"id"`
	UserID uint `gorm:"index" json:"-"`

	Symbol string  `json:"symbol"`
	Side   string  `json:"side"` // BUY / SELL
	Qty    float64 `json:"qty"`

	Entry float64 `json:"entry"`
	Exit  float64 `json:"exit"`
	Fees  float64 `json:"fees"`
	PnL   float64 `json:"pnl"`

	Date time.Time `json:"date"`

	Account  string `json:"account"` // Paper / Live
	Session  string `json:"session"` // Asia / London / NY
	Strategy string `json:"strategy"`

	Journal Journal `json:"journal"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
