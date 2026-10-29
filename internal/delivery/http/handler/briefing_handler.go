package handler

import (
	"net/http"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// BriefingHandler handles HTTP requests for previewing and manually triggering briefings.
type BriefingHandler struct {
	briefingUC domain.BriefingUsecase
	logger     *zap.Logger
}

// NewBriefingHandler creates a new BriefingHandler instance.
func NewBriefingHandler(briefingUC domain.BriefingUsecase, logger *zap.Logger) *BriefingHandler {
	return &BriefingHandler{
		briefingUC: briefingUC,
		logger:     logger,
	}
}

// GetDailyBriefing returns the text content of today's morning briefing.
func (h *BriefingHandler) GetDailyBriefing(c echo.Context) error {
	msg, err := h.briefingUC.GetDailyBriefing(c.Request().Context())
	if err != nil {
		h.logger.Error("failed to get daily briefing", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"briefing": msg})
}

// SendDailyBriefing generates and dispatches the morning briefing via WhatsApp.
func (h *BriefingHandler) SendDailyBriefing(c echo.Context) error {
	if err := h.briefingUC.SendDailyBriefing(c.Request().Context()); err != nil {
		h.logger.Error("failed to send daily briefing", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "daily briefing sent"})
}

// GetNightlyDebrief returns the text content of tonight's debrief.
func (h *BriefingHandler) GetNightlyDebrief(c echo.Context) error {
	msg, err := h.briefingUC.GetNightlyDebrief(c.Request().Context())
	if err != nil {
		h.logger.Error("failed to get nightly debrief", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"debrief": msg})
}

// SendNightlyDebrief generates and dispatches the nightly debrief via WhatsApp.
func (h *BriefingHandler) SendNightlyDebrief(c echo.Context) error {
	if err := h.briefingUC.SendNightlyDebrief(c.Request().Context()); err != nil {
		h.logger.Error("failed to send nightly debrief", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "nightly debrief sent"})
}

