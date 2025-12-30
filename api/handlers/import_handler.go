package handlers

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"jourt_backend/internal/models"
	"jourt_backend/pkg/middleware"
)

type ImportHandler struct {
	db *gorm.DB
}

func NewImportHandler(db *gorm.DB) *ImportHandler {
	return &ImportHandler{db: db}
}

// POST /import/csv (multipart/form-data)
// fields: file (required), mapping (optional JSON), account(optional), session(optional)
func (h *ImportHandler) ImportCSV(c *gin.Context) {
	userID := middleware.GetUserID(c)

	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}

	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if ext != ".csv" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only .csv is supported for now"})
		return
	}

	file, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open uploaded file"})
		return
	}
	defer file.Close()

	// Optional: account/session override for imported trades
	account := strings.TrimSpace(c.PostForm("account"))
	if account == "" {
		account = "Paper"
	}
	session := strings.TrimSpace(c.PostForm("session"))

	// Optional mapping JSON (from your frontend mapping UI)
	mapping := map[string]string{}
	if m := strings.TrimSpace(c.PostForm("mapping")); m != "" {
		if err := json.Unmarshal([]byte(m), &mapping); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mapping json"})
			return
		}
	}

	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true

	// Read header row
	headers, err := reader.Read()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "csv is empty or invalid"})
		return
	}

	// Build header index map
	headerIndex := map[string]int{}
	for i, h := range headers {
		headerIndex[strings.TrimSpace(h)] = i
	}

	// If mapping not provided, auto-map from headers (best-effort)
	if len(mapping) == 0 {
		mapping = autoMapHeaders(headers)
	}

	// Validate required mapping keys exist
	required := []string{"date", "symbol", "side", "qty", "entry", "exit", "fees", "pnl"}
	for _, k := range required {
		if strings.TrimSpace(mapping[k]) == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "mapping incomplete",
				"missing": k,
				"hint":    "provide mapping JSON from frontend or rename CSV headers",
			})
			return
		}
		// make sure header exists in file
		if _, ok := headerIndex[mapping[k]]; !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":  "mapped header not found in csv",
				"field":  k,
				"header": mapping[k],
			})
			return
		}
	}

	// Parse rows -> trades
	var trades []models.Trade
	var rowErrors []gin.H

	rowNum := 1 // header is row 1
	for {
		rowNum++
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			addRowErr(&rowErrors, rowNum, "csv read error: "+err.Error())
			continue
		}

		tr, ok, msg := normalizeCSVRow(userID, row, headerIndex, mapping, account, session)
		if !ok {
			addRowErr(&rowErrors, rowNum, msg)
			continue
		}

		trades = append(trades, tr)

		// safety cap (optional): prevent someone uploading 2 million rows by accident
		if len(trades) > 200000 {
			addRowErr(&rowErrors, rowNum, "too many rows (cap reached)")
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

	// Save to DB in a transaction
	var inserted int
	err = h.db.Transaction(func(tx *gorm.DB) error {
		// Create import job
		job := models.ImportJob{
			UserID:     userID,
			SourceFile: fh.Filename,
			Broker:     "", // optional later
			RowsCount:  len(trades),
		}
		if err := tx.Create(&job).Error; err != nil {
			return err
		}

		// Bulk insert trades
		// (CreateInBatches avoids timeouts for large files)
		if err := tx.CreateInBatches(&trades, 500).Error; err != nil {
			return err
		}
		inserted = len(trades)

		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to import trades", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":       true,
		"inserted": inserted,
		"errors":   rowErrors, // only first few
		"mapping":  mapping,
	})
}

func addRowErr(list *[]gin.H, row int, msg string) {
	// limit returned errors (avoid huge responses)
	if len(*list) >= 25 {
		return
	}
	*list = append(*list, gin.H{"row": row, "error": msg})
}

func autoMapHeaders(headers []string) map[string]string {
	lower := make([]string, len(headers))
	for i, h := range headers {
		lower[i] = strings.ToLower(strings.TrimSpace(h))
	}

	pick := func(keys ...string) string {
		for i, h := range lower {
			for _, k := range keys {
				if strings.Contains(h, k) {
					return strings.TrimSpace(headers[i])
				}
			}
		}
		return ""
	}

	return map[string]string{
		"date":   pick("time", "date", "filled", "executed"),
		"symbol": pick("symbol", "instrument", "ticker", "product"),
		"side":   pick("side", "buy/sell", "direction", "type"),
		"qty":    pick("qty", "quantity", "size", "volume", "units"),
		"entry":  pick("entry", "avg entry", "open price", "entry price", "price"),
		"exit":   pick("exit", "avg exit", "close price", "exit price"),
		"fees":   pick("fee", "fees", "commission", "swap"),
		"pnl":    pick("pnl", "profit", "pl", "net pnl"),
	}
}

func normalizeCSVRow(
	userID uint,
	row []string,
	headerIndex map[string]int,
	mapping map[string]string,
	account string,
	session string,
) (models.Trade, bool, string) {

	get := func(field string) string {
		h := mapping[field]
		i := headerIndex[h]
		if i < 0 || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	// Parse date (supports a few common formats)
	dateStr := get("date")
	tm, ok := parseTimeFlexible(dateStr)
	if !ok {
		return models.Trade{}, false, "invalid date/time: " + dateStr
	}

	side := strings.ToUpper(get("side"))
	if side == "LONG" {
		side = "BUY"
	}
	if side == "SHORT" {
		side = "SELL"
	}
	if side != "BUY" && side != "SELL" {
		// still accept common values
		if strings.Contains(side, "BUY") {
			side = "BUY"
		} else if strings.Contains(side, "SELL") {
			side = "SELL"
		} else {
			return models.Trade{}, false, "invalid side: " + side
		}
	}

	symbol := get("symbol")
	if symbol == "" {
		return models.Trade{}, false, "missing symbol"
	}

	qty := parseNum(get("qty"))
	entry := parseNum(get("entry"))
	exit := parseNum(get("exit"))
	fees := parseNum(get("fees"))
	pnl := parseNum(get("pnl"))

	tr := models.Trade{
		UserID:  userID,
		Date:    tm,
		Symbol:  symbol,
		Side:    side,
		Qty:     qty,
		Entry:   entry,
		Exit:    exit,
		Fees:    fees,
		PnL:     pnl,
		Account: account,
		Session: session,
	}

	return tr, true, ""
}

func parseNum(v string) float64 {
	// remove currency, commas, etc.
	clean := strings.ReplaceAll(v, ",", "")
	clean = strings.TrimSpace(clean)
	clean = strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '.' || r == '-' {
			return r
		}
		return -1
	}, clean)

	if clean == "" || clean == "-" {
		return 0
	}

	f, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0
	}
	return f
}
