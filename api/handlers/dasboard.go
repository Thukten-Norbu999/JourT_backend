// handlers/dashboard_handler.go
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

type DashboardHandler struct {
	db *gorm.DB
}

func NewDashboardHandler(db *gorm.DB) *DashboardHandler {
	return &DashboardHandler{db: db}
}

// GET /api/dashboard?range=30d
// Range options: 7d, 30d, 90d, 365d, all
func (h *DashboardHandler) GetDashboard(c *gin.Context) {
	userID := middleware.GetUserID(c)
	rangeParam := c.DefaultQuery("range", "30d")

	// Calculate date range
	now := time.Now()
	var from time.Time

	switch rangeParam {
	case "7d":
		from = now.AddDate(0, 0, -7)
	case "30d":
		from = now.AddDate(0, 0, -30)
	case "90d":
		from = now.AddDate(0, 0, -90)
	case "365d":
		from = now.AddDate(0, -12, 0)
	case "all":
		from = time.Time{} // Zero time = fetch all
	default:
		from = now.AddDate(0, 0, -30)
	}

	// Fetch all trades in range
	var trades []models.Trade
	query := h.db.Where("user_id = ?", userID)

	if !from.IsZero() {
		query = query.Where("date >= ?", from)
	}

	if err := query.Order("date ASC").Find(&trades).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch trades"})
		return
	}

	// Calculate statistics
	stats := calculateStats(trades)

	// Build equity curve
	equityCurve := buildEquityCurve(trades)

	// Get recent trades (last 10)
	recentTrades := getRecentTrades(trades, 10)

	c.JSON(http.StatusOK, gin.H{
		"stats":        stats,
		"equityCurve":  equityCurve,
		"recentTrades": recentTrades,
	})
}

// Stats represents dashboard statistics
type Stats struct {
	TotalPnl     float64 `json:"totalPnl"`
	TotalTrades  int     `json:"totalTrades"`
	WinRate      float64 `json:"winRate"`
	AvgWin       float64 `json:"avgWin"`
	AvgLoss      float64 `json:"avgLoss"`
	ProfitFactor float64 `json:"profitFactor"`
	MaxDrawdown  float64 `json:"maxDrawdown"`
	Streak       Streak  `json:"streak"`
	GrossProfit  float64 `json:"grossProfit"`
	GrossLoss    float64 `json:"grossLoss"`
	TotalFees    float64 `json:"totalFees"`
	NetPnl       float64 `json:"netPnl"`
	Wins         int     `json:"wins"`
	Losses       int     `json:"losses"`
	Breakeven    int     `json:"breakeven"`
}

type Streak struct {
	Current int `json:"current"`
	Best    int `json:"best"`
}

type EquityPoint struct {
	Date   string  `json:"date"`
	Equity float64 `json:"equity"`
}

type RecentTrade struct {
	ID     uint      `json:"id"`
	Date   time.Time `json:"date"`
	Symbol string    `json:"symbol"`
	Side   string    `json:"side"`
	Qty    float64   `json:"qty"`
	Entry  float64   `json:"entry"`
	Exit   float64   `json:"exit"`
	PnL    float64   `json:"pnl"`
	Fees   float64   `json:"fees"`
}

func calculateStats(trades []models.Trade) Stats {
	stats := Stats{}

	if len(trades) == 0 {
		return stats
	}

	totalPnl := 0.0
	grossProfit := 0.0
	grossLoss := 0.0
	totalFees := 0.0
	wins := 0
	losses := 0
	breakeven := 0

	currentStreak := 0
	bestStreak := 0
	lastWasWin := false

	// Calculate running equity for drawdown
	runningEquity := 0.0
	peak := 0.0
	maxDrawdown := 0.0

	for _, t := range trades {
		pnl := t.PnL
		totalPnl += pnl
		totalFees += t.Fees

		// Update running equity
		runningEquity += pnl
		if runningEquity > peak {
			peak = runningEquity
		}
		drawdown := runningEquity - peak
		if drawdown < maxDrawdown {
			maxDrawdown = drawdown
		}

		// Win/Loss counting
		if pnl > 0 {
			wins++
			grossProfit += pnl

			// Streak tracking
			if lastWasWin {
				currentStreak++
			} else {
				currentStreak = 1
				lastWasWin = true
			}

			if currentStreak > bestStreak {
				bestStreak = currentStreak
			}
		} else if pnl < 0 {
			losses++
			grossLoss += pnl
			lastWasWin = false
			currentStreak = 0
		} else {
			breakeven++
			lastWasWin = false
			currentStreak = 0
		}
	}

	stats.TotalPnl = totalPnl
	stats.TotalFees = totalFees
	stats.NetPnl = totalPnl - totalFees
	stats.TotalTrades = len(trades)
	stats.Wins = wins
	stats.Losses = losses
	stats.Breakeven = breakeven
	stats.GrossProfit = grossProfit
	stats.GrossLoss = grossLoss
	stats.MaxDrawdown = maxDrawdown

	// Win rate
	if stats.TotalTrades > 0 {
		stats.WinRate = float64(wins) / float64(stats.TotalTrades)
	}

	// Average win/loss
	if wins > 0 {
		stats.AvgWin = grossProfit / float64(wins)
	}
	if losses > 0 {
		stats.AvgLoss = grossLoss / float64(losses)
	}

	// Profit factor
	if grossLoss != 0 {
		stats.ProfitFactor = grossProfit / -grossLoss
	}

	// Streak
	if lastWasWin {
		stats.Streak.Current = currentStreak
	} else {
		stats.Streak.Current = 0
	}
	stats.Streak.Best = bestStreak

	return stats
}

func buildEquityCurve(trades []models.Trade) []EquityPoint {
	if len(trades) == 0 {
		return []EquityPoint{}
	}

	// Group trades by date
	type DayData struct {
		Date string
		PnL  float64
	}

	dayMap := make(map[string]float64)

	for _, t := range trades {
		dateStr := t.Date.Format("2006-01-02")
		dayMap[dateStr] += t.PnL
	}

	// Convert to sorted slice
	days := make([]DayData, 0, len(dayMap))
	for date, pnl := range dayMap {
		days = append(days, DayData{Date: date, PnL: pnl})
	}

	sort.Slice(days, func(i, j int) bool {
		return days[i].Date < days[j].Date
	})

	// Build cumulative equity curve
	equity := 0.0
	equityCurve := make([]EquityPoint, 0, len(days))

	for _, day := range days {
		equity += day.PnL
		equityCurve = append(equityCurve, EquityPoint{
			Date:   day.Date,
			Equity: equity,
		})
	}

	return equityCurve
}

func getRecentTrades(trades []models.Trade, limit int) []RecentTrade {
	if len(trades) == 0 {
		return []RecentTrade{}
	}

	// Sort by date descending (most recent first)
	sorted := make([]models.Trade, len(trades))
	copy(sorted, trades)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Date.After(sorted[j].Date)
	})

	// Take only the limit
	if len(sorted) > limit {
		sorted = sorted[:limit]
	}

	// Convert to response format
	recent := make([]RecentTrade, len(sorted))
	for i, t := range sorted {
		recent[i] = RecentTrade{
			ID:     t.ID,
			Date:   t.Date,
			Symbol: t.Symbol,
			Side:   t.Side,
			Qty:    t.Qty,
			Entry:  t.Entry,
			Exit:   t.Exit,
			PnL:    t.PnL,
			Fees:   t.Fees,
		}
	}

	return recent
}

// Register routes
// In your main router setup:
/*
func SetupRoutes(r *gin.Engine, db *gorm.DB) {
	dashboardHandler := handlers.NewDashboardHandler(db)

	api := r.Group("/api")
	api.Use(middleware.Auth())
	{
		api.GET("/dashboard", dashboardHandler.GetDashboard)
	}
}
*/
