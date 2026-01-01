package handlers

import (
	"net/http"
	"sort"
	"strings"
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

// calculateTradePnL calculates PnL if missing or updates if entry/exit changed
func calculateTradePnL(t *models.Trade) float64 {
	// If PnL exists and is reasonable, return it
	if t.PnL != 0 {
		return t.PnL
	}

	// Calculate based on side
	var pnl float64
	side := strings.ToUpper(t.Side)

	if side == "BUY" || side == "LONG" {
		// Long position: profit when exit > entry
		pnl = (t.Exit - t.Entry) * t.Qty
	} else if side == "SELL" || side == "SHORT" {
		// Short position: profit when entry > exit
		pnl = (t.Entry - t.Exit) * t.Qty
	}

	// Subtract fees
	pnl -= t.Fees

	return pnl
}

// ensureTradesPnL ensures all trades have PnL calculated
func ensureTradesPnL(trades []models.Trade) []models.Trade {
	for i := range trades {
		if trades[i].PnL == 0 && trades[i].Entry != 0 && trades[i].Exit != 0 {
			trades[i].PnL = calculateTradePnL(&trades[i])
		}
	}
	return trades
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

	// Validate date range
	if to.Before(from) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "to date must be after from date"})
		return
	}

	// Limit to 1 year max to prevent performance issues
	if to.Sub(from).Hours() > 365*24 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date range cannot exceed 1 year"})
		return
	}

	// Include the whole "to" day
	toInclusive := to.Add(24*time.Hour - time.Nanosecond)

	var trades []models.Trade
	if err := h.db.
		Where("user_id = ? AND date >= ? AND date <= ?", userID, from, toInclusive).
		Order("date ASC").
		Find(&trades).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch trades"})
		return
	}

	// Ensure all trades have PnL calculated
	trades = ensureTradesPnL(trades)

	total := 0.0
	wins := 0
	losses := 0
	breakeven := 0
	fees := 0.0
	grossProfit := 0.0
	grossLoss := 0.0

	for _, t := range trades {
		total += t.PnL
		fees += t.Fees

		if t.PnL > 0 {
			wins++
			grossProfit += t.PnL
		} else if t.PnL < 0 {
			losses++
			grossLoss += t.PnL
		} else {
			breakeven++
		}
	}

	count := len(trades)
	winRate := 0.0
	avgWin := 0.0
	avgLoss := 0.0
	profitFactor := 0.0

	if count > 0 {
		winRate = float64(wins) / float64(count)
	}
	if wins > 0 {
		avgWin = grossProfit / float64(wins)
	}
	if losses > 0 {
		avgLoss = grossLoss / float64(losses)
	}
	if grossLoss != 0 {
		profitFactor = grossProfit / -grossLoss
	}

	c.JSON(http.StatusOK, gin.H{
		"from":         from.Format("2006-01-02"),
		"to":           to.Format("2006-01-02"),
		"trades":       count,
		"wins":         wins,
		"losses":       losses,
		"breakeven":    breakeven,
		"winRate":      winRate, // 0..1
		"totalPnl":     total,
		"totalFees":    fees,
		"netPnl":       total - fees,
		"grossProfit":  grossProfit,
		"grossLoss":    grossLoss,
		"avgWin":       avgWin,
		"avgLoss":      avgLoss,
		"profitFactor": profitFactor,
	})
}

// GET /api/pnl/calendar?month=YYYY-MM
// Returns days sorted by date
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
		Order("date ASC").
		Find(&trades).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch trades"})
		return
	}

	// Ensure all trades have PnL calculated
	trades = ensureTradesPnL(trades)

	type Day struct {
		Date   string  `json:"date"`
		PnL    float64 `json:"pnl"`
		Trades int     `json:"trades"`
		Fees   float64 `json:"fees"`
		Wins   int     `json:"wins"`
		Losses int     `json:"losses"`
	}
	m := map[string]*Day{}

	for _, t := range trades {
		d := t.Date.Format("2006-01-02")
		if m[d] == nil {
			m[d] = &Day{Date: d}
		}
		m[d].PnL += t.PnL
		m[d].Fees += t.Fees
		m[d].Trades++

		if t.PnL > 0 {
			m[d].Wins++
		} else if t.PnL < 0 {
			m[d].Losses++
		}
	}

	out := make([]Day, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })

	// Calculate monthly totals
	monthlyPnl := 0.0
	monthlyFees := 0.0
	monthlyTrades := 0
	monthlyWins := 0
	monthlyLosses := 0

	for _, d := range out {
		monthlyPnl += d.PnL
		monthlyFees += d.Fees
		monthlyTrades += d.Trades
		monthlyWins += d.Wins
		monthlyLosses += d.Losses
	}

	c.JSON(http.StatusOK, gin.H{
		"month": month,
		"days":  out,
		"summary": gin.H{
			"totalPnl":    monthlyPnl,
			"totalFees":   monthlyFees,
			"totalTrades": monthlyTrades,
			"wins":        monthlyWins,
			"losses":      monthlyLosses,
			"winRate":     calculateWinRate(monthlyWins, monthlyLosses),
		},
	})
}

// GET /api/pnl/daily?date=YYYY-MM-DD
// Get detailed trades for a specific day
func (h *PnLHandler) DailyDetail(c *gin.Context) {
	userID := middleware.GetUserID(c)

	dateStr := c.Query("date")
	if dateStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date parameter is required"})
		return
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date must be YYYY-MM-DD"})
		return
	}

	nextDay := date.AddDate(0, 0, 1)

	var trades []models.Trade
	if err := h.db.
		Where("user_id = ? AND date >= ? AND date < ?", userID, date, nextDay).
		Order("date ASC").
		Find(&trades).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch trades"})
		return
	}

	// Ensure all trades have PnL calculated
	trades = ensureTradesPnL(trades)

	totalPnl := 0.0
	totalFees := 0.0
	wins := 0
	losses := 0

	for _, t := range trades {
		totalPnl += t.PnL
		totalFees += t.Fees
		if t.PnL > 0 {
			wins++
		} else if t.PnL < 0 {
			losses++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"date":   dateStr,
		"trades": trades,
		"summary": gin.H{
			"count":     len(trades),
			"totalPnl":  totalPnl,
			"totalFees": totalFees,
			"netPnl":    totalPnl - totalFees,
			"wins":      wins,
			"losses":    losses,
			"winRate":   calculateWinRate(wins, losses),
		},
	})
}

// GET /api/pnl/stats
// Get overall trading statistics
func (h *PnLHandler) Stats(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var trades []models.Trade
	if err := h.db.
		Where("user_id = ?", userID).
		Order("date ASC").
		Find(&trades).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch trades"})
		return
	}

	if len(trades) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"totalTrades": 0,
			"message":     "No trades found",
		})
		return
	}

	// Ensure all trades have PnL calculated
	trades = ensureTradesPnL(trades)

	// Calculate statistics
	totalPnl := 0.0
	totalFees := 0.0
	wins := 0
	losses := 0
	grossProfit := 0.0
	grossLoss := 0.0
	largestWin := 0.0
	largestLoss := 0.0

	dayMap := make(map[string]float64)

	for _, t := range trades {
		totalPnl += t.PnL
		totalFees += t.Fees

		if t.PnL > 0 {
			wins++
			grossProfit += t.PnL
			if t.PnL > largestWin {
				largestWin = t.PnL
			}
		} else if t.PnL < 0 {
			losses++
			grossLoss += t.PnL
			if t.PnL < largestLoss {
				largestLoss = t.PnL
			}
		}

		// Track daily PnL for day-level stats
		dateKey := t.Date.Format("2006-01-02")
		dayMap[dateKey] += t.PnL
	}

	winningDays := 0
	losingDays := 0
	for _, pnl := range dayMap {
		if pnl > 0 {
			winningDays++
		} else if pnl < 0 {
			losingDays++
		}
	}

	count := len(trades)
	avgPnl := totalPnl / float64(count)
	avgWin := 0.0
	avgLoss := 0.0
	profitFactor := 0.0

	if wins > 0 {
		avgWin = grossProfit / float64(wins)
	}
	if losses > 0 {
		avgLoss = grossLoss / float64(losses)
	}
	if grossLoss != 0 {
		profitFactor = grossProfit / -grossLoss
	}

	c.JSON(http.StatusOK, gin.H{
		"totalTrades":  count,
		"wins":         wins,
		"losses":       losses,
		"winRate":      calculateWinRate(wins, losses),
		"totalPnl":     totalPnl,
		"totalFees":    totalFees,
		"netPnl":       totalPnl - totalFees,
		"avgPnl":       avgPnl,
		"grossProfit":  grossProfit,
		"grossLoss":    grossLoss,
		"avgWin":       avgWin,
		"avgLoss":      avgLoss,
		"profitFactor": profitFactor,
		"largestWin":   largestWin,
		"largestLoss":  largestLoss,
		"tradingDays":  len(dayMap),
		"winningDays":  winningDays,
		"losingDays":   losingDays,
		"dayWinRate":   calculateWinRate(winningDays, losingDays),
	})
}

// Helper function
func calculateWinRate(wins, losses int) float64 {
	total := wins + losses
	if total == 0 {
		return 0.0
	}
	return float64(wins) / float64(total)
}
