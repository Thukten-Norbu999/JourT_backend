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

type TradeHandler struct {
	db *gorm.DB
}

func NewTradeHandler(db *gorm.DB) *TradeHandler {
	return &TradeHandler{db: db}
}

// GET /api/trades?date=YYYY-MM-DD (optional)
func (h *TradeHandler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)

	q := h.db.
		Preload("Journal.Psychology").
		Preload("Journal.Metrics").
		Preload("Journal.Screenshots").
		Where("user_id = ?", userID)

	// optional filter by day (for calendar clicks)
	if d := strings.TrimSpace(c.Query("date")); d != "" {
		day, err := time.ParseInLocation("2006-01-02", d, time.Local)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "date must be YYYY-MM-DD"})
			return
		}
		start := day
		end := day.Add(24 * time.Hour)
		q = q.Where("date >= ? AND date < ?", start, end)
	}

	var trades []models.Trade
	if err := q.Order("date desc").Find(&trades).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch trades"})
		return
	}

	// return array directly (frontend-friendly)
	c.JSON(http.StatusOK, trades)
}

// POST /api/trades
//
// Frontend payload:
//
//	{
//	  "trade": { date, symbol, side, qty, entry, exit, fees, account, session, notes },
//	  "journal": {...},
//	  "psychology": {...},
//	  "metrics": {...}
//	}
//
// POST /api/trades
func (h *TradeHandler) Create(c *gin.Context) {
	userID := middleware.GetUserID(c)

	// ✅ matches AddTradeModal payload
	var req struct {
		Trade struct {
			Date    string   `json:"date" binding:"required"` // ISO string from toISOString()
			Symbol  string   `json:"symbol" binding:"required"`
			Side    string   `json:"side" binding:"required"` // BUY/SELL
			Qty     float64  `json:"qty" binding:"required"`
			Entry   *float64 `json:"entry"`   // null allowed
			Exit    *float64 `json:"exit"`    // null allowed
			Fees    float64  `json:"fees"`    // frontend sends 0 if blank
			Account string   `json:"account"` // Paper/Live
			Session *string  `json:"session"` // null allowed
			Notes   *string  `json:"notes"`   // null allowed (if your model supports it)
		} `json:"trade" binding:"required"`

		Journal *struct {
			SetupID  *string  `json:"setupId"` // frontend sends setupId; your DB journal uses Setup string for now
			Thesis   string   `json:"thesis"`
			Mistakes string   `json:"mistakes"`
			Lessons  string   `json:"lessons"`
			Rating   int      `json:"rating"`
			Tags     []string `json:"tags"`
		} `json:"journal"`

		Psychology *struct {
			Emotion        int             `json:"emotion"`
			Confidence     int             `json:"confidence"`
			ExecutionScore int             `json:"executionScore"`
			PsyTags        []string        `json:"psyTags"`
			Rules          map[string]bool `json:"rules"`
			Discipline     int             `json:"discipline"`
		} `json:"psychology"`

		Metrics *struct {
			RMultiple *float64 `json:"rMultiple"`
			MAE       *float64 `json:"mae"`
			MFE       *float64 `json:"mfe"`
		} `json:"metrics"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid request",
			"details": err.Error(),
		})
		return
	}

	// ✅ time parsing: your frontend uses ISO (RFC3339) -> this works
	tm, ok := parseTimeFlexible(req.Trade.Date)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "trade.date must be RFC3339 (ISO) or common formats"})
		return
	}

	side := strings.ToUpper(strings.TrimSpace(req.Trade.Side))
	if side == "LONG" {
		side = "BUY"
	}
	if side == "SHORT" {
		side = "SELL"
	}
	if side != "BUY" && side != "SELL" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "trade.side must be BUY or SELL"})
		return
	}

	symbol := strings.TrimSpace(req.Trade.Symbol)
	if symbol == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "trade.symbol is required"})
		return
	}
	if req.Trade.Qty <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "trade.qty must be > 0"})
		return
	}

	account := strings.TrimSpace(req.Trade.Account)
	if account == "" {
		account = "Paper"
	}

	entry := 0.0
	if req.Trade.Entry != nil {
		entry = *req.Trade.Entry
	}
	exit := 0.0
	if req.Trade.Exit != nil {
		exit = *req.Trade.Exit
	}

	session := ""
	if req.Trade.Session != nil {
		session = strings.TrimSpace(*req.Trade.Session)
	}

	// ✅ create Trade first
	trade := models.Trade{
		UserID:  userID,
		Date:    tm,
		Symbol:  symbol,
		Side:    side,
		Qty:     req.Trade.Qty,
		Entry:   entry,
		Exit:    exit,
		Fees:    req.Trade.Fees,
		PnL:     0, // optional: compute later if you want
		Account: account,
		Session: session,
		// Strategy: "", // if your model has it
		// Notes:    "", // only if your model has Notes
	}

	// ✅ transaction: trade + optional journal/psy/metrics
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&trade).Error; err != nil {
			return err
		}

		// Optional journal payload
		if req.Journal != nil {
			j := models.Journal{
				TradeID:  trade.ID,
				Setup:    "",
				Thesis:   req.Journal.Thesis,
				Mistakes: req.Journal.Mistakes,
				Lessons:  req.Journal.Lessons,
				Rating:   req.Journal.Rating,
				Tags:     joinCSV(req.Journal.Tags),
			}

			// ⚠️ your frontend sends setupId, but your Journal model currently stores Setup as string.
			// For now store the setupId string in Setup. Later you can migrate to SetupID uint.
			if req.Journal.SetupID != nil {
				j.Setup = strings.TrimSpace(*req.Journal.SetupID)
			}

			if err := tx.Create(&j).Error; err != nil {
				return err
			}

			// Optional psychology
			if req.Psychology != nil {
				psy := models.Psychology{
					JournalID:      j.ID,
					Emotion:        req.Psychology.Emotion,
					Confidence:     req.Psychology.Confidence,
					ExecutionScore: req.Psychology.ExecutionScore,
					Discipline:     req.Psychology.Discipline,
					PsyTags:        joinCSV(req.Psychology.PsyTags),

					FollowedPlan:       req.Psychology.Rules != nil && req.Psychology.Rules["followedPlan"],
					RespectedRisk:      req.Psychology.Rules != nil && req.Psychology.Rules["respectedRisk"],
					WaitedConfirmation: req.Psychology.Rules != nil && req.Psychology.Rules["waitedConfirmation"],
					NoRevenge:          req.Psychology.Rules != nil && req.Psychology.Rules["noRevenge"],
				}
				if err := tx.Create(&psy).Error; err != nil {
					return err
				}
			}

			// Optional metrics
			if req.Metrics != nil {
				m := models.Metrics{
					JournalID: j.ID,
				}
				if req.Metrics.RMultiple != nil {
					m.RMultiple = *req.Metrics.RMultiple
				}
				if req.Metrics.MAE != nil {
					m.MAE = *req.Metrics.MAE
				}
				if req.Metrics.MFE != nil {
					m.MFE = *req.Metrics.MFE
				}
				if err := tx.Create(&m).Error; err != nil {
					return err
				}
			}
		}

		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create trade", "details": err.Error()})
		return
	}

	// ✅ return created trade w/ preloads
	var out models.Trade
	_ = h.db.
		Preload("Journal.Psychology").
		Preload("Journal.Metrics").
		Preload("Journal.Screenshots").
		First(&out, trade.ID).Error

	c.JSON(http.StatusCreated, out)
}

// DELETE /api/trades/:id
func (h *TradeHandler) Delete(c *gin.Context) {
	userID := middleware.GetUserID(c)
	id := c.Param("id")

	if err := h.db.Where("id = ? AND user_id = ?", id, userID).Delete(&models.Trade{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete trade"})
		return
	}

	c.Status(http.StatusNoContent)
}

// PUT /api/trades/:id/journal  (UPSERT: create journal if missing)
//
// Accepts frontend payload shape:
//
//	{
//	  setup, thesis, mistakes, lessons, rating, tags:[],
//	  psychology:{ emotion, confidence, executionScore, psyTags:[], rules:{...}, discipline },
//	  metrics:{ rMultiple, mae, mfe },
//	  screenshots:{ before, after }
//	}
func (h *TradeHandler) UpsertJournal(c *gin.Context) {
	userID := middleware.GetUserID(c)
	id := c.Param("id")

	// Allow both tags: []string and tags: "a,b"
	var req struct {
		Setup    string   `json:"setup"`
		Thesis   string   `json:"thesis"`
		Mistakes string   `json:"mistakes"`
		Lessons  string   `json:"lessons"`
		Rating   int      `json:"rating"`
		Tags     []string `json:"tags"`
		TagsRaw  string   `json:"tagsRaw"` // optional if you ever send it

		Psychology struct {
			Emotion        int             `json:"emotion"`
			Confidence     int             `json:"confidence"`
			ExecutionScore int             `json:"executionScore"`
			PsyTags        []string        `json:"psyTags"`
			Rules          map[string]bool `json:"rules"`
			Discipline     int             `json:"discipline"`
		} `json:"psychology"`

		Metrics struct {
			RMultiple float64 `json:"rMultiple"`
			MAE       float64 `json:"mae"`
			MFE       float64 `json:"mfe"`
		} `json:"metrics"`

		Screenshots struct {
			Before string `json:"before"`
			After  string `json:"after"`
		} `json:"screenshots"`

		// Back-compat: if some client still sends `tags` as string
		TagsString string `json:"tagsString"`
		TagsCSV    string `json:"tagsCsv"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	var trade models.Trade
	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&trade).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "trade not found"})
		return
	}

	// normalize tags sources
	tags := req.Tags
	if len(tags) == 0 {
		raw := strings.TrimSpace(req.TagsRaw)
		if raw == "" {
			raw = strings.TrimSpace(req.TagsString)
		}
		if raw == "" {
			raw = strings.TrimSpace(req.TagsCSV)
		}
		if raw != "" {
			tags = splitCSV(raw)
		}
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		// Find or create journal
		var j models.Journal
		jErr := tx.Where("trade_id = ?", trade.ID).First(&j).Error
		if jErr != nil {
			if jErr == gorm.ErrRecordNotFound {
				j = models.Journal{TradeID: trade.ID}
				if err := tx.Create(&j).Error; err != nil {
					return err
				}
			} else {
				return jErr
			}
		}

		// Update journal fields
		j.Setup = strings.TrimSpace(req.Setup)
		j.Thesis = req.Thesis
		j.Mistakes = req.Mistakes
		j.Lessons = req.Lessons
		j.Rating = req.Rating
		j.Tags = joinCSV(tags)

		if err := tx.Save(&j).Error; err != nil {
			return err
		}

		// Upsert psychology (by journal_id)
		{
			var psy models.Psychology
			_ = tx.Where("journal_id = ?", j.ID).First(&psy).Error

			psy.JournalID = j.ID
			psy.Emotion = req.Psychology.Emotion
			psy.Confidence = req.Psychology.Confidence
			psy.ExecutionScore = req.Psychology.ExecutionScore
			psy.Discipline = req.Psychology.Discipline
			psy.PsyTags = joinCSV(req.Psychology.PsyTags)

			if req.Psychology.Rules != nil {
				psy.FollowedPlan = req.Psychology.Rules["followedPlan"]
				psy.RespectedRisk = req.Psychology.Rules["respectedRisk"]
				psy.WaitedConfirmation = req.Psychology.Rules["waitedConfirmation"]
				psy.NoRevenge = req.Psychology.Rules["noRevenge"]
			}

			if err := tx.Save(&psy).Error; err != nil {
				return err
			}
		}

		// Upsert metrics (by journal_id)
		{
			var m models.Metrics
			_ = tx.Where("journal_id = ?", j.ID).First(&m).Error

			m.JournalID = j.ID
			m.RMultiple = req.Metrics.RMultiple
			m.MAE = req.Metrics.MAE
			m.MFE = req.Metrics.MFE

			if err := tx.Save(&m).Error; err != nil {
				return err
			}
		}

		// Upsert screenshots (by journal_id)
		{
			var s models.Screenshots
			_ = tx.Where("journal_id = ?", j.ID).First(&s).Error

			s.JournalID = j.ID
			s.Before = req.Screenshots.Before
			s.After = req.Screenshots.After

			if err := tx.Save(&s).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save journal", "details": err.Error()})
		return
	}

	// return updated trade with preloads
	var out models.Trade
	if err := h.db.
		Preload("Journal.Psychology").
		Preload("Journal.Metrics").
		Preload("Journal.Screenshots").
		First(&out, trade.ID).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	c.JSON(http.StatusOK, out)
}

func parseTimeFlexible(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}

	// RFC3339 handles timezone/offsets properly
	if tm, err := time.Parse(time.RFC3339, s); err == nil {
		return tm, true
	}

	// Common broker/local formats (assume server local time)
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, f := range formats {
		if tm, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return tm, true
		}
	}

	return time.Time{}, false
}

func joinCSV(items []string) string {
	out := ""
	for i, s := range items {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if out == "" {
			out = s
		} else {
			out += "," + s
		}
		if i > 200 {
			break
		}
	}
	return out
}

func splitCSV(s string) []string {
	raw := strings.Split(s, ",")
	out := make([]string, 0, len(raw))
	for _, x := range raw {
		t := strings.TrimSpace(x)
		if t != "" {
			out = append(out, t)
		}
	}
	if len(out) > 50 {
		out = out[:50]
	}
	return out
}
