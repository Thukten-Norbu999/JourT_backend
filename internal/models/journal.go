package models

import (
	"gorm.io/gorm"
)

type Journal struct {
	gorm.Model
	ID      uint `gorm:"primaryKey" json:"id"`
	TradeID uint `gorm:"uniqueIndex" json:"tradeId"`

	Setup    string `json:"setup"`
	Thesis   string `json:"thesis"`
	Mistakes string `json:"mistakes"`
	Lessons  string `json:"lessons"`
	Rating   int    `json:"rating"`

	Tags string `json:"tags"` // CSV string

	Psychology  Psychology  `json:"psychology"`
	Metrics     Metrics     `json:"metrics"`
	Screenshots Screenshots `json:"screenshots"`
}
