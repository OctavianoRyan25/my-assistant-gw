package handler

import (
	"net/http"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// StatsHandler aggregates statistics across all domains.
type StatsHandler struct {
	profileUC   domain.UserProfileUsecase
	budgetUC    domain.BudgetUsecase
	expenseUC   domain.ExpenseUsecase
	nutritionUC domain.NutritionUsecase
	workoutUC   domain.WorkoutUsecase
	reminderUC  domain.ReminderUsecase
	logger      *zap.Logger
	timezone    *time.Location
}

// NewStatsHandler creates a new StatsHandler.
func NewStatsHandler(
	profileUC domain.UserProfileUsecase,
	budgetUC domain.BudgetUsecase,
	expenseUC domain.ExpenseUsecase,
	nutritionUC domain.NutritionUsecase,
	workoutUC domain.WorkoutUsecase,
	reminderUC domain.ReminderUsecase,
	logger *zap.Logger,
	timezone *time.Location,
) *StatsHandler {
	return &StatsHandler{
		profileUC:   profileUC,
		budgetUC:    budgetUC,
		expenseUC:   expenseUC,
		nutritionUC: nutritionUC,
		workoutUC:   workoutUC,
		reminderUC:  reminderUC,
		logger:      logger,
		timezone:    timezone,
	}
}

// OverviewResponse bundles overall stats for the dashboard.
type OverviewResponse struct {
	Timestamp      string                        `json:"timestamp"`
	Profile        *domain.UserProfile           `json:"profile,omitempty"`
	BMR            float64                       `json:"bmr"`
	TDEE           float64                       `json:"tdee"`
	Budget         *domain.BudgetStatus          `json:"budget,omitempty"`
	ExpenseReport  *domain.ExpenseReport         `json:"expense_report,omitempty"`
	RecentExpenses []*domain.Expense             `json:"recent_expenses,omitempty"`
	NutritionToday *domain.DailyNutritionSummary `json:"nutrition_today,omitempty"`
	WorkoutStats   *domain.WorkoutMonthlyStats   `json:"workout_stats,omitempty"`
	RecentWorkouts []*domain.Workout             `json:"recent_workouts,omitempty"`
	ActiveReminders []*domain.Reminder           `json:"active_reminders,omitempty"`
}

// GetOverview returns a comprehensive snapshot of all personal data.
// GET /api/v1/stats/overview
func (h *StatsHandler) GetOverview(c echo.Context) error {
	ctx := c.Request().Context()
	now := time.Now().In(h.timezone)
	year, month := now.Year(), int(now.Month())

	resp := OverviewResponse{
		Timestamp: now.Format(time.RFC3339),
	}

	// 1. User Profile & BMR/TDEE
	if h.profileUC != nil {
		if profile, err := h.profileUC.GetProfile(ctx); err == nil && profile != nil {
			resp.Profile = profile
			resp.BMR, resp.TDEE = h.profileUC.CalculateBMRAndTDEE(profile)
		}
	}

	// 2. Budget status
	if h.budgetUC != nil {
		if budgetStatus, err := h.budgetUC.GetBudgetStatus(ctx, year, month); err == nil {
			resp.Budget = budgetStatus
		}
	}

	// 3. Expenses
	if h.expenseUC != nil {
		if report, err := h.expenseUC.GetMonthlyReport(ctx, year, month); err == nil {
			resp.ExpenseReport = report
		}
		if details, err := h.expenseUC.GetDetailMonthlyReport(ctx, year, month); err == nil {
			// Limit to recent 20 for overview
			if len(details) > 20 {
				resp.RecentExpenses = details[:20]
			} else {
				resp.RecentExpenses = details
			}
		}
	}

	// 4. Nutrition today
	if h.nutritionUC != nil {
		if summary, err := h.nutritionUC.GetDailySummary(ctx, now); err == nil {
			resp.NutritionToday = summary
		}
	}

	// 5. Workout
	if h.workoutUC != nil {
		if stats, err := h.workoutUC.GetCurrentMonthStats(ctx); err == nil {
			resp.WorkoutStats = stats
		}
		if recent, err := h.workoutUC.GetRecentWorkouts(ctx, 15); err == nil {
			resp.RecentWorkouts = recent
		}
	}

	// 6. Reminders
	if h.reminderUC != nil {
		if reminders, err := h.reminderUC.ListActive(ctx); err == nil {
			resp.ActiveReminders = reminders
		}
	}

	return c.JSON(http.StatusOK, echo.Map{
		"success": true,
		"data":    resp,
	})
}

// GetNutritionToday returns today's nutrition summary and meal breakdown.
// GET /api/v1/nutrition/today
func (h *StatsHandler) GetNutritionToday(c echo.Context) error {
	ctx := c.Request().Context()
	now := time.Now().In(h.timezone)

	dateStr := c.QueryParam("date")
	if dateStr != "" {
		if parsedDate, err := time.ParseInLocation("2006-01-02", dateStr, h.timezone); err == nil {
			now = parsedDate
		}
	}

	if h.nutritionUC == nil {
		return c.JSON(http.StatusNotImplemented, echo.Map{"success": false, "error": "nutrition usecase not initialized"})
	}

	summary, err := h.nutritionUC.GetDailySummary(ctx, now)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"success": false, "error": err.Error()})
	}

	return c.JSON(http.StatusOK, echo.Map{
		"success": true,
		"data":    summary,
	})
}

// GetWorkouts returns workout stats and logs.
// GET /api/v1/workouts
func (h *StatsHandler) GetWorkouts(c echo.Context) error {
	ctx := c.Request().Context()
	if h.workoutUC == nil {
		return c.JSON(http.StatusNotImplemented, echo.Map{"success": false, "error": "workout usecase not initialized"})
	}

	stats, err := h.workoutUC.GetCurrentMonthStats(ctx)
	if err != nil {
		h.logger.Warn("failed to get workout stats", zap.Error(err))
	}

	recent, err := h.workoutUC.GetRecentWorkouts(ctx, 30)
	if err != nil {
		h.logger.Warn("failed to get recent workouts", zap.Error(err))
	}

	return c.JSON(http.StatusOK, echo.Map{
		"success": true,
		"data": echo.Map{
			"stats":  stats,
			"recent": recent,
		},
	})
}

