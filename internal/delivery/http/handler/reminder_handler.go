package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
	"github.com/OctavianoRyan25/my-assistant-gw/internal/usecase"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// ReminderHandler handles HTTP endpoints for reminder management.
type ReminderHandler struct {
	reminderUC domain.ReminderUsecase
	logger     *zap.Logger
	timezone   *time.Location
}

// NewReminderHandler creates a new ReminderHandler.
func NewReminderHandler(reminderUC domain.ReminderUsecase, logger *zap.Logger, timezone *time.Location) *ReminderHandler {
	return &ReminderHandler{reminderUC: reminderUC, logger: logger, timezone: timezone}
}

type createReminderRequest struct {
	Title       string  `json:"title"`
	ScheduledAt string  `json:"scheduled_at"` // format: "2006-01-02 15:04:05"
	Recurrence  *string `json:"recurrence,omitempty"`
}

type cancelReminderRequest struct {
	ID int64 `json:"id"`
}

// ListActive returns all active reminders.
// GET /api/v1/reminders
func (h *ReminderHandler) ListActive(c echo.Context) error {
	reminders, err := h.reminderUC.ListActive(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"success": false, "error": err.Error()})
	}
	return c.JSON(http.StatusOK, echo.Map{"success": true, "data": reminders, "formatted": usecase.FormatReminderList(reminders)})
}

// Create creates a new reminder via API.
// POST /api/v1/reminders
func (h *ReminderHandler) Create(c echo.Context) error {
	var req createReminderRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"success": false, "error": "invalid request body"})
	}
	if req.Title == "" || req.ScheduledAt == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"success": false, "error": "title and scheduled_at are required"})
	}

	scheduledAt, err := time.ParseInLocation("2006-01-02 15:04:05", req.ScheduledAt, h.timezone)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"success": false, "error": "invalid scheduled_at format, use YYYY-MM-DD HH:MM:SS"})
	}

	reminder, err := h.reminderUC.CreateReminder(c.Request().Context(), req.Title, scheduledAt, req.Recurrence)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"success": false, "error": err.Error()})
	}
	return c.JSON(http.StatusCreated, echo.Map{"success": true, "data": reminder})
}

// Cancel cancels a reminder by ID.
// DELETE /api/v1/reminders/:id
func (h *ReminderHandler) Cancel(c echo.Context) error {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"success": false, "error": "invalid id"})
	}
	if err := h.reminderUC.Cancel(c.Request().Context(), id); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"success": false, "error": err.Error()})
	}
	return c.JSON(http.StatusOK, echo.Map{"success": true, "message": "reminder cancelled"})
}
