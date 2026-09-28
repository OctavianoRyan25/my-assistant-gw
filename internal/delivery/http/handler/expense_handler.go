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

// ExpenseHandler handles HTTP endpoints for expense management.
type ExpenseHandler struct {
	expenseUC domain.ExpenseUsecase
	logger    *zap.Logger
	timezone  *time.Location
}

// NewExpenseHandler creates a new ExpenseHandler.
func NewExpenseHandler(expenseUC domain.ExpenseUsecase, logger *zap.Logger, timezone *time.Location) *ExpenseHandler {
	return &ExpenseHandler{expenseUC: expenseUC, logger: logger, timezone: timezone}
}

type createExpenseRequest struct {
	Amount      float64 `json:"amount"`
	Category    string  `json:"category"`
	Description string  `json:"description"`
	OccurredAt  string  `json:"occurred_at,omitempty"` // optional, format YYYY-MM-DD HH:MM:SS
}

// Record creates a new expense entry.
// POST /api/v1/expenses
func (h *ExpenseHandler) Record(c echo.Context) error {
	var req createExpenseRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"success": false, "error": "invalid request body"})
	}
	if req.Amount <= 0 || req.Description == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"success": false, "error": "amount and description are required"})
	}

	occurredAt := time.Now().In(h.timezone)
	if req.OccurredAt != "" {
		t, err := time.ParseInLocation("2006-01-02 15:04:05", req.OccurredAt, h.timezone)
		if err != nil {
			return c.JSON(http.StatusBadRequest, echo.Map{"success": false, "error": "invalid occurred_at format"})
		}
		occurredAt = t
	}

	expense, err := h.expenseUC.RecordExpense(c.Request().Context(), req.Amount, req.Category, req.Description, occurredAt)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"success": false, "error": err.Error()})
	}
	return c.JSON(http.StatusCreated, echo.Map{"success": true, "data": expense, "message": usecase.FormatExpenseConfirmation(expense)})
}

// GetMonthlyReport returns expense report for a given month.
// GET /api/v1/expenses/report?year=2026&month=9
func (h *ExpenseHandler) GetMonthlyReport(c echo.Context) error {
	now := time.Now().In(h.timezone)
	year := now.Year()
	month := int(now.Month())

	if y := c.QueryParam("year"); y != "" {
		if parsed, err := strconv.Atoi(y); err == nil {
			year = parsed
		}
	}
	if m := c.QueryParam("month"); m != "" {
		if parsed, err := strconv.Atoi(m); err == nil && parsed >= 1 && parsed <= 12 {
			month = parsed
		}
	}

	report, err := h.expenseUC.GetMonthlyReport(c.Request().Context(), year, month)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"success": false, "error": err.Error()})
	}
	return c.JSON(http.StatusOK, echo.Map{"success": true, "data": report, "formatted": usecase.FormatExpenseReport(report)})
}
