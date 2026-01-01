package api

import (
	"jourt_backend/api/handlers"
	"jourt_backend/pkg/config"
	"jourt_backend/pkg/middleware"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetupRoutes(r *gin.Engine, db *gorm.DB) {
	r.Use(config.SetupCORS())
	r.GET("/healthz", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	auth := handlers.NewAuthHandler(db)
	r.POST("/auth/register", auth.RegisterUser)
	r.POST("/auth/login", auth.LoginUser)
	r.POST("/auth/logout", auth.LogoutUser)
	r.GET("/auth/me", auth.Me)

	// protected
	api := r.Group("/api")
	api.Use(middleware.RequireAuth())

	dashboardH := handlers.NewDashboardHandler(db)
	api.GET("/dashboard", dashboardH.GetDashboard)

	setupH := handlers.NewSetupHandler(db)
	api.GET("/setups", setupH.List)
	api.POST("/setups", setupH.Create)

	tradeH := handlers.NewTradeHandler(db)
	api.GET("/trades", tradeH.List)
	api.POST("/trade", tradeH.Create)
	api.DELETE("/trades/:id", tradeH.Delete)

	journalH := handlers.NewJournalHandler(db)
	api.PUT("/trades/:id/journal", journalH.Upsert)

	pnlH := handlers.NewPnLHandler(db)
	api.GET("/pnl/summary", pnlH.Summary)
	api.GET("/pnl/calendar", pnlH.Calendar)

	importH := handlers.NewImportHandler(db)
	api.POST("/import/csv", importH.ImportCSV)

	backtestH := handlers.NewBacktestHandler(db)
	api.POST("/backtest/run", backtestH.Run)

}
