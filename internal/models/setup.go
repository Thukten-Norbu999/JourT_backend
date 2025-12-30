package models

import "gorm.io/gorm"

type Setup struct {
	gorm.Model
	ID     uint `gorm:"primaryKey" json:"id"`
	UserID uint `gorm:"index" json:"-"`

	Name        string `json:"name"`
	Description string `json:"description"`
}
