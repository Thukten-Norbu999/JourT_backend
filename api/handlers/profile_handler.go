package handlers

import (
	"gorm.io/gorm"
)

type ProfileHandler struct {
	db *gorm.DB
}
