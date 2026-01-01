// api/handlers/import_handler.go
package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"jourt_backend/internal/models"
	"jourt_backend/pkg/middleware"
)

/*
Frontend sends JSON like:

POST /api/import/csv
{
  "sourceFile": "my.csv",
  "mapping": {...},
  "options": { "filledOnly": true, "singlePriceMode": true, "preferClosingTime": true },
  "trades": [
    {
      "date": "2025-12-31T01:23:45.000Z",
      "symbol": "EURUSD",
      "side": "BUY",
      "qty": 1,
      "entry": 1.1,
      "exit": 1.2,
      "fees": 0.1,
      "pnl": 10,
      "meta": { ... }
    }
  ],
  "account": "Paper",
  "session": "NY"
}
*/

type ImportHandler struct {
	db *gorm.DB
}

func NewImportHandler(db *gorm.DB) *ImportHandler {
	return &ImportHandler{db: db}
}

type ImportOptions struct {
	FilledOnly        bool `json:"filledOnly"`
	SinglePriceMode   bool `json:"singlePriceMode"`
	PreferClosingTime bool `json:"preferClosingTime"`
}

type ImportTrade struct {
	Date   string  `json:"date"`
	Symbol string  `json:"symbol"`
	Side   string  `json:"side"`
	Qty    float64 `json:"qty"`

	Entry float64 `json:"entry"`
	Exit  float64 `json:"exit"`
	Fees  float64 `json:"fees"`
	PnL   float64 `json:"pnl"`

	Meta map[string]any `json:"meta"`
}

type ImportRequest struct {
	SourceFile string            `json:"sourceFile"`
	Mapping    map[string]string `json:"mapping"`
	Options    ImportOptions     `json:"options"`
	Trades     []ImportTrade     `json:"trades"`

	Account string `json:"account"`
	Session string `json:"session"`
}

// POST /api/import/csv (JSON)
func (h *ImportHandler) ImportCSV(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var req ImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid request json",
			"details": err.Error(),
		})
		return
	}

	if len(req.Trades) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no trades provided"})
		return
	}

	account := strings.TrimSpace(req.Account)
	if account == "" {
		account = "Paper"
	}
	session := strings.TrimSpace(req.Session)

	trades := make([]models.Trade, 0, len(req.Trades))
	rowErrors := make([]gin.H, 0, 8)

	for i, t := range req.Trades {
		tr, ok, msg := normalizeImportTrade(userID, t, account, session)
		if !ok {
			addRowErrIndex(&rowErrors, i, msg)
			continue
		}
		trades = append(trades, tr)

		// safety cap
		if len(trades) > 200000 {
			addRowErrIndex(&rowErrors, i, "too many rows (cap reached)")
			break
		}
	}

	if len(trades) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":     "no valid trades found",
			"rowErrors": rowErrors,
		})
		return
	}

	var inserted int
	err := h.db.Transaction(func(tx *gorm.DB) error {
		src := strings.TrimSpace(req.SourceFile)
		if src == "" {
			src = "import.json"
		}

		job := models.ImportJob{
			UserID:     userID,
			SourceFile: src,
			Broker:     "",
			RowsCount:  len(trades),
		}
		if err := tx.Create(&job).Error; err != nil {
			return err
		}

		if err := tx.CreateInBatches(&trades, 500).Error; err != nil {
			return err
		}

		inserted = len(trades)
		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to import trades",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":       true,
		"inserted": inserted,
		"errors":   rowErrors,   // capped
		"mapping":  req.Mapping, // echo for debugging
		"options":  req.Options, // echo for debugging
		"source":   req.SourceFile,
	})
}

func addRowErrIndex(list *[]gin.H, idx int, msg string) {
	if len(*list) >= 25 {
		return
	}
	*list = append(*list, gin.H{
		"index": idx + 1, // 1-based
		"error": msg,
	})
}

func normalizeImportTrade(userID uint, in ImportTrade, account, session string) (models.Trade, bool, string) {
	dateStr := strings.TrimSpace(in.Date)
	tm, ok := parseTimeFlexibleImport(dateStr)
	if !ok {
		return models.Trade{}, false, "invalid date/time: " + dateStr
	}

	side := strings.ToUpper(strings.TrimSpace(in.Side))
	switch side {
	case "LONG":
		side = "BUY"
	case "SHORT":
		side = "SELL"
	}
	if side != "BUY" && side != "SELL" {
		if strings.Contains(side, "BUY") {
			side = "BUY"
		} else if strings.Contains(side, "SELL") {
			side = "SELL"
		} else {
			return models.Trade{}, false, "invalid side: " + strings.TrimSpace(in.Side)
		}
	}

	symbol := strings.TrimSpace(in.Symbol)
	if symbol == "" {
		return models.Trade{}, false, "missing symbol"
	}

	if in.Qty <= 0 {
		return models.Trade{}, false, "qty must be > 0"
	}

	tr := models.Trade{
		UserID:  userID,
		Date:    tm,
		Symbol:  symbol,
		Side:    side,
		Qty:     in.Qty,
		Entry:   in.Entry,
		Exit:    in.Exit,
		Fees:    in.Fees,
		PnL:     in.PnL,
		Account: account,
		Session: session,
	}

	return tr, true, ""
}

// self-contained (won't conflict with any existing parseTimeFlexible you may have elsewhere)
func parseTimeFlexibleImport(s string) (time.Time, bool) {
	str := strings.TrimSpace(s)
	if str == "" {
		return time.Time{}, false
	}

	// Most common from frontend: ISO with Z
	if t, err := time.Parse(time.RFC3339Nano, str); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, str); err == nil {
		return t, true
	}

	// Common "no timezone" formats (assume local)
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02",
		"02/01/2006 15:04:05", // DD/MM/YYYY
		"02/01/2006 15:04",
		"02/01/2006",
		"01/02/2006 15:04:05", // MM/DD/YYYY
		"01/02/2006 15:04",
		"01/02/2006",
		"Jan 2, 2006 15:04:05",
		"Jan 2, 2006 15:04",
		"Jan 2, 2006",
	}

	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, str,
			time.Local); err == nil {
			return t, true
		}
	}

	// last attempt: Date.parse-like inputs that sometimes still parse
	if t, err := time.ParseInLocation(time.ANSIC, str, time.Local); err == nil {
		return t, true
	}

	return time.Time{}, false
}
