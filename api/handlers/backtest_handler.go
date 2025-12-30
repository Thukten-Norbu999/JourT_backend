package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"jourt_backend/pkg/middleware"
)

type BacktestHandler struct {
	db *gorm.DB
}

func NewBacktestHandler(db *gorm.DB) *BacktestHandler {
	return &BacktestHandler{db: db}
}

// POST /backtest/run
func (h *BacktestHandler) Run(c *gin.Context) {
	_ = middleware.GetUserID(c)

	var req struct {
		Strategy  string         `json:"strategy"`
		Symbol    string         `json:"symbol"`
		Timeframe string         `json:"timeframe"`
		From      string         `json:"from"`
		To        string         `json:"to"`
		Params    map[string]any `json:"params"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	// TEMP: return mock-like result shape (matches your frontend usage)
	c.JSON(http.StatusOK, gin.H{
		"summary": gin.H{
			"trades":       10,
			"winRate":      0.55,
			"totalPnl":     123.45,
			"profitFactor": 1.42,
			"maxDrawdown":  -78.9,
		},
		"equity": []gin.H{
			{"equity": 1000},
			{"equity": 1010},
			{"equity": 990},
		},
		"trades": []gin.H{},
	})
}
