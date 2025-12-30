package models

import "gorm.io/gorm"

type Screenshots struct {
	gorm.Model
	ID        uint `gorm:"primaryKey" json:"id"`
	JournalID uint `gorm:"uniqueIndex" json:"journalId"`

	Before string `json:"before"`
	After  string `json:"after"`
}
