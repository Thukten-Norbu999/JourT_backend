package models

import "gorm.io/gorm"

type Metrics struct {
	gorm.Model
	ID        uint `gorm:"primaryKey" json:"id"`
	JournalID uint `gorm:"uniqueIndex" json:"journalId"`

	RMultiple float64 `json:"rMultiple"`
	MAE       float64 `json:"mae"`
	MFE       float64 `json:"mfe"`
}
