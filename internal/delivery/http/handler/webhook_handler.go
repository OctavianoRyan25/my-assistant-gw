package handler

import (
	"net/http"
	"strconv"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// WebhookHandler handles incoming WhatsApp-style webhook messages.
// In production, this will be replaced by whatsmeow event listener.
type WebhookHandler struct {
	chatUC domain.ChatUsecase
	logger *zap.Logger
}

// NewWebhookHandler creates a new WebhookHandler.
func NewWebhookHandler(chatUC domain.ChatUsecase, logger *zap.Logger) *WebhookHandler {
	return &WebhookHandler{chatUC: chatUC, logger: logger}
}

// incomingMessage represents the request payload from WhatsApp webhook.
type incomingMessage struct {
	From    string `json:"from"`    // sender JID
	Message string `json:"message"` // plain text message
}

// messageResponse is the standard API response.
type messageResponse struct {
	Success bool   `json:"success"`
	Reply   string `json:"reply,omitempty"`
	Error   string `json:"error,omitempty"`
}

// HandleIncoming processes an incoming message and returns the assistant reply.
// POST /api/v1/webhook/message
func (h *WebhookHandler) HandleIncoming(c echo.Context) error {
	var req incomingMessage
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, messageResponse{Success: false, Error: "invalid request body"})
	}
	if req.Message == "" {
		return c.JSON(http.StatusBadRequest, messageResponse{Success: false, Error: "message is required"})
	}

	h.logger.Info("incoming message", zap.String("from", req.From), zap.String("message", req.Message))

	reply, err := h.chatUC.HandleMessage(c.Request().Context(), req.Message)
	if err != nil {
		h.logger.Error("handle message error", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, messageResponse{Success: false, Error: err.Error()})
	}

	return c.JSON(http.StatusOK, messageResponse{Success: true, Reply: reply})
}

// GetHistory returns recent chat history.
// GET /api/v1/chat/history?limit=20
func (h *WebhookHandler) GetHistory(c echo.Context) error {
	limitStr := c.QueryParam("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	history, err := h.chatUC.GetHistory(c.Request().Context(), limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, messageResponse{Success: false, Error: err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "data": history})
}

// ResetHistory clears the conversation context.
// POST /api/v1/chat/reset
func (h *WebhookHandler) ResetHistory(c echo.Context) error {
	if err := h.chatUC.ResetContext(c.Request().Context()); err != nil {
		return c.JSON(http.StatusInternalServerError, messageResponse{Success: false, Error: err.Error()})
	}
	return c.JSON(http.StatusOK, messageResponse{Success: true, Reply: "Chat history cleared."})
}
