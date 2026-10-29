package domain

import (
	"context"
	"time"
)

// DailyBriefing holds information aggregated for the morning proactive briefing.
type DailyBriefing struct {
	Date             time.Time              `json:"date"`
	TodayReminders   []*Reminder            `json:"today_reminders"`
	OverdueReminders []*Reminder            `json:"overdue_reminders"`
	BudgetStatus     *BudgetStatus          `json:"budget_status,omitempty"`
	SafeDailySpend   float64                `json:"safe_daily_spend"`
	DaysRemaining    int                    `json:"days_remaining"`
	Profile          *UserProfile           `json:"profile,omitempty"`
	NutritionSummary *DailyNutritionSummary `json:"nutrition_summary,omitempty"`
}

// NightlyDebrief holds information aggregated for the evening recap.
type NightlyDebrief struct {
	Date             time.Time              `json:"date"`
	TodayExpenses    []*Expense             `json:"today_expenses"`
	ExpenseReport    *ExpenseReport         `json:"expense_report,omitempty"`
	BudgetStatus     *BudgetStatus          `json:"budget_status,omitempty"`
	NutritionSummary *DailyNutritionSummary `json:"nutrition_summary,omitempty"`
	Workouts         []*Workout             `json:"workouts"`
	TotalBurnedKcal  int                    `json:"total_burned_kcal"`
	TotalDurationMin int                    `json:"total_duration_min"`
	PendingReminders []*Reminder            `json:"pending_reminders"`
}

// BriefingUsecase defines business logic for generating and sending proactive daily briefings and nightly debriefs.
type BriefingUsecase interface {
	GetDailyBriefing(ctx context.Context) (string, error)
	SendDailyBriefing(ctx context.Context) error
	GetNightlyDebrief(ctx context.Context) (string, error)
	SendNightlyDebrief(ctx context.Context) error
}

