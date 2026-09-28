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
}
