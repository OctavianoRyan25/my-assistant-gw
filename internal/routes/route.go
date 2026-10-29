package routes

import (
	"github.com/OctavianoRyan25/my-assistant-gw/internal/delivery/http/handler"
	"github.com/labstack/echo/v4"
)

// Handlers bundles all HTTP handlers for dependency injection.
type Handlers struct {
	WebhookHandler  *handler.WebhookHandler
	ReminderHandler *handler.ReminderHandler
	ExpenseHandler  *handler.ExpenseHandler
	ProfileHandler  *handler.ProfileHandler
	StatsHandler    *handler.StatsHandler
	BriefingHandler *handler.BriefingHandler
}

// RegisterRoutes wires all API routes to their respective handlers.
func RegisterRoutes(e *echo.Echo, h *Handlers) {
	api := e.Group("/api/v1")

	// Health check
	api.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{"status": "ok"})
	})

	// Webhook — receives messages from WhatsApp (or test client)
	webhook := api.Group("/webhook")
	webhook.POST("/message", h.WebhookHandler.HandleIncoming)

	// Chat
	chat := api.Group("/chat")
	chat.GET("/history", h.WebhookHandler.GetHistory)
	chat.POST("/reset", h.WebhookHandler.ResetHistory)

	// Reminders
	reminders := api.Group("/reminders")
	reminders.GET("", h.ReminderHandler.ListActive)
	reminders.POST("", h.ReminderHandler.Create)
	reminders.DELETE("/:id", h.ReminderHandler.Cancel)

	// Expenses
	expenses := api.Group("/expenses")
	expenses.POST("", h.ExpenseHandler.Record)
	expenses.GET("/report", h.ExpenseHandler.GetMonthlyReport)
	expenses.GET("/detail", h.ExpenseHandler.GetDetailMonthlyReport)

	// Profile
	if h.ProfileHandler != nil {
		profile := api.Group("/profile")
		profile.GET("", h.ProfileHandler.Get)
		profile.PUT("", h.ProfileHandler.Update)
	}

	// Stats & Aggregations
	if h.StatsHandler != nil {
		stats := api.Group("/stats")
		stats.GET("/overview", h.StatsHandler.GetOverview)

		nutrition := api.Group("/nutrition")
		nutrition.GET("/today", h.StatsHandler.GetNutritionToday)

		workouts := api.Group("/workouts")
		workouts.GET("", h.StatsHandler.GetWorkouts)
	}

	// Proactive Briefings
	if h.BriefingHandler != nil {
		briefing := api.Group("/briefing")
		briefing.GET("/daily", h.BriefingHandler.GetDailyBriefing)
		briefing.POST("/daily/send", h.BriefingHandler.SendDailyBriefing)
		briefing.GET("/nightly", h.BriefingHandler.GetNightlyDebrief)
		briefing.POST("/nightly/send", h.BriefingHandler.SendNightlyDebrief)
	}
}
