package models

import (
	"time"

	"gorm.io/gorm"
)

type ImportJob struct {
	gorm.Model
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"index" json:"-"`
	SourceFile string    `json:"sourceFile"`
	Broker     string    `json:"broker"`
	RowsCount  int       `json:"rowsCount"`
	CreatedAt  time.Time `json:"createdAt"`
}
