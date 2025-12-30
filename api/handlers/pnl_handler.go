package handlers

import (
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"jourt_backend/internal/models"
	"jourt_backend/pkg/middleware"
)

type PnLHandler struct {
	db *gorm.DB
}

func NewPnLHandler(db *gorm.DB) *PnLHandler {
	return &PnLHandler{db: db}
}

// GET /api/pnl/summary?from=YYYY-MM-DD&to=YYYY-MM-DD
// Defaults: last 30 days
func (h *PnLHandler) Summary(c *gin.Context) {
	userID := middleware.GetUserID(c)

	fromS := c.Query("from")
	toS := c.Query("to")

	now := time.Now()
	from := now.AddDate(0, 0, -30)
	to := now

	var err error
	if fromS != "" {
		from, err = time.Parse("2006-01-02", fromS)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from must be YYYY-MM-DD"})
			return
		}
	}
	if toS != "" {
		to, err = time.Parse("2006-01-02", toS)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "to must be YYYY-MM-DD"})
			return
		}
	}

	// include the whole "to" day (avoid off-by-one when user sends a date)
	toInclusive := to.Add(24*time.Hour - time.Nanosecond)

	var trades []models.Trade
	if err := h.db.
		Where("user_id = ? AND date >= ? AND date <= ?", userID, from, toInclusive).
		Find(&trades).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch trades"})
		return
	}

	total := 0.0
	wins := 0
	losses := 0
	fees := 0.0

	for _, t := range trades {
		total += t.PnL
		fees += t.Fees
		if t.PnL > 0 {
			wins++
		} else if t.PnL < 0 {
			losses++
		}
	}

	count := len(trades)
	winRate := 0.0
	if count > 0 {
		winRate = float64(wins) / float64(count)
	}

	c.JSON(http.StatusOK, gin.H{
		"from":      from.Format("2006-01-02"),
		"to":        to.Format("2006-01-02"),
		"trades":    count,
		"wins":      wins,
		"losses":    losses,
		"winRate":   winRate, // 0..1
		"totalPnl":  total,
		"totalFees": fees,
	})
}

// GET /api/pnl/calendar?month=YYYY-MM
// Returns days sorted by date (so frontend calendar is stable)
func (h *PnLHandler) Calendar(c *gin.Context) {
	userID := middleware.GetUserID(c)

	month := c.Query("month") // YYYY-MM
	if month == "" {
		month = time.Now().Format("2006-01")
	}

	start, err := time.Parse("2006-01", month)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "month must be YYYY-MM"})
		return
	}
	end := start.AddDate(0, 1, 0)

	var trades []models.Trade
	if err := h.db.
		Where("user_id = ? AND date >= ? AND date < ?", userID, start, end).
		Find(&trades).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch trades"})
		return
	}

	type Day struct {
		Date  string  `json:"date"`
		PnL   float64 `json:"pnl"`
		Count int     `json:"count"`
		Fees  float64 `json:"fees"`
	}
	m := map[string]*Day{}

	for _, t := range trades {
		d := t.Date.Format("2006-01-02")
		if m[d] == nil {
			m[d] = &Day{Date: d}
		}
		m[d].PnL += t.PnL
		m[d].Fees += t.Fees
		m[d].Count++
	}

	out := make([]Day, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })

	c.JSON(http.StatusOK, gin.H{
		"month": month,
		"days":  out,
	})
}
