package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"jourt_backend/internal/models"
	"jourt_backend/pkg/middleware"
)

type JournalHandler struct {
	db *gorm.DB
}

func NewJournalHandler(db *gorm.DB) *JournalHandler {
	return &JournalHandler{db: db}
}

// PUT /api/trades/:id/journal
// Upsert Journal + Psychology + Metrics + Screenshots (all optional)
func (h *JournalHandler) Upsert(c *gin.Context) {
	userID := middleware.GetUserID(c)
	tradeID := c.Param("id")

	// ✅ Allow "optional" blocks so CSV-imported trades (no journal) are fine
	type reqBody struct {
		Setup    string   `json:"setup"`
		Thesis   string   `json:"thesis"`
		Mistakes string   `json:"mistakes"`
		Lessons  string   `json:"lessons"`
		Rating   int      `json:"rating"`
		Tags     []string `json:"tags"`

		Psychology *struct {
			Emotion        int             `json:"emotion"`
			Confidence     int             `json:"confidence"`
			ExecutionScore int             `json:"executionScore"`
			PsyTags        []string        `json:"psyTags"`
			Rules          map[string]bool `json:"rules"`
			Discipline     int             `json:"discipline"`
		} `json:"psychology,omitempty"`

		Metrics *struct {
			RMultiple float64 `json:"rMultiple"`
			MAE       float64 `json:"mae"`
			MFE       float64 `json:"mfe"`
		} `json:"metrics,omitempty"`

		Screenshots *struct {
			Before string `json:"before"`
			After  string `json:"after"`
		} `json:"screenshots,omitempty"`
	}

	var req reqBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	// Make sure trade belongs to user
	var trade models.Trade
	if err := h.db.Where("id = ? AND user_id = ?", tradeID, userID).First(&trade).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "trade not found"})
		return
	}

	// ✅ Transaction so we don't partially save (journal updated but psychology failed etc.)
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		// Find or create Journal
		var journal models.Journal
		err := tx.Where("trade_id = ?", trade.ID).First(&journal).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				journal = models.Journal{TradeID: trade.ID}
				if err := tx.Create(&journal).Error; err != nil {
					return err
				}
			} else {
				return err
			}
		}

		// Update basic journal fields (these are safe to always set)
		journal.Setup = strings.TrimSpace(req.Setup)
		journal.Thesis = strings.TrimSpace(req.Thesis)
		journal.Mistakes = strings.TrimSpace(req.Mistakes)
		journal.Lessons = strings.TrimSpace(req.Lessons)
		journal.Rating = req.Rating
		journal.Tags = joinCSV(req.Tags)

		if err := tx.Save(&journal).Error; err != nil {
			return err
		}

		// Upsert Psychology (only if provided)
		if req.Psychology != nil {
			var psy models.Psychology
			pErr := tx.Where("journal_id = ?", journal.ID).First(&psy).Error
			if pErr != nil && !errors.Is(pErr, gorm.ErrRecordNotFound) {
				return pErr
			}

			psy.JournalID = journal.ID
			psy.Emotion = req.Psychology.Emotion
			psy.Confidence = req.Psychology.Confidence
			psy.ExecutionScore = req.Psychology.ExecutionScore
			psy.Discipline = req.Psychology.Discipline
			psy.PsyTags = joinCSV(req.Psychology.PsyTags)

			rules := req.Psychology.Rules
			if rules == nil {
				rules = map[string]bool{}
			}
			psy.FollowedPlan = rules["followedPlan"]
			psy.RespectedRisk = rules["respectedRisk"]
			psy.WaitedConfirmation = rules["waitedConfirmation"]
			psy.NoRevenge = rules["noRevenge"]

			if err := tx.Save(&psy).Error; err != nil {
				return err
			}
		}

		// Upsert Metrics (only if provided)
		if req.Metrics != nil {
			var m models.Metrics
			mErr := tx.Where("journal_id = ?", journal.ID).First(&m).Error
			if mErr != nil && !errors.Is(mErr, gorm.ErrRecordNotFound) {
				return mErr
			}

			m.JournalID = journal.ID
			m.RMultiple = req.Metrics.RMultiple
			m.MAE = req.Metrics.MAE
			m.MFE = req.Metrics.MFE

			if err := tx.Save(&m).Error; err != nil {
				return err
			}
		}

		// Upsert Screenshots (only if provided)
		if req.Screenshots != nil {
			var s models.Screenshots
			sErr := tx.Where("journal_id = ?", journal.ID).First(&s).Error
			if sErr != nil && !errors.Is(sErr, gorm.ErrRecordNotFound) {
				return sErr
			}

			s.JournalID = journal.ID
			s.Before = strings.TrimSpace(req.Screenshots.Before)
			s.After = strings.TrimSpace(req.Screenshots.After)

			if err := tx.Save(&s).Error; err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save journal", "details": err.Error()})
		return
	}

	// Return full trade with preloads (journal can be nil if none, but we just upserted)
	var out models.Trade
	if err := h.db.
		Preload("Journal.Psychology").
		Preload("Journal.Metrics").
		Preload("Journal.Screenshots").
		First(&out, trade.ID).Error; err != nil {
		// shouldn't happen, but don't crash UX
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	c.JSON(http.StatusOK, gin.H{"trade": out})
}

// func joinCSV(items []string) string {
// 	// Simple join with trimming + cap
// 	out := make([]string, 0, len(items))
// 	for _, s := range items {
// 		s = strings.TrimSpace(s)
// 		if s == "" {
// 			continue
// 		}
// 		out = append(out, s)
// 		if len(out) >= 200 {
// 			break
// 		}
// 	}
// 	return strings.Join(out, ",")
// }
