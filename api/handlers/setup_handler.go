package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"jourt_backend/internal/models"
	"jourt_backend/pkg/middleware"
)

type SetupHandler struct {
	db *gorm.DB
}

func NewSetupHandler(db *gorm.DB) *SetupHandler {
	return &SetupHandler{db: db}
}

func (h *SetupHandler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var setups []models.Setup
	if err := h.db.Where("user_id = ?", userID).Order("name asc").Find(&setups).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch setups"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"setups": setups})
}

func (h *SetupHandler) Create(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	s := models.Setup{
		UserID:      userID,
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
	}

	if err := h.db.Create(&s).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create setup"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"setup": s})
}
