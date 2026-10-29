package usecase_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
	"github.com/OctavianoRyan25/my-assistant-gw/internal/usecase"
	"go.uber.org/zap"
)

// Mock Reminder Repository
type mockReminderRepo struct {
	activeReminders []*domain.Reminder
}

func (m *mockReminderRepo) Create(ctx context.Context, reminder *domain.Reminder) error { return nil }
func (m *mockReminderRepo) GetByID(ctx context.Context, id int64) (*domain.Reminder, error) {
	return nil, nil
}
func (m *mockReminderRepo) GetLatest(ctx context.Context) (*domain.Reminder, error) { return nil, nil }
func (m *mockReminderRepo) GetAllActive(ctx context.Context) ([]*domain.Reminder, error) {
	return m.activeReminders, nil
}
func (m *mockReminderRepo) UpdateStatus(ctx context.Context, id int64, status string) error {
	return nil
}
func (m *mockReminderRepo) Update(ctx context.Context, reminder *domain.Reminder) error { return nil }
func (m *mockReminderRepo) Delete(ctx context.Context, id int64) error                  { return nil }
func (m *mockReminderRepo) GetDueReminders(ctx context.Context, now time.Time) ([]*domain.Reminder, error) {
	return nil, nil
}

// Mock Expense Usecase
type mockExpenseUC struct {
	todayExpenses []*domain.Expense
	todayReport   *domain.ExpenseReport
}

func (m *mockExpenseUC) RecordExpense(ctx context.Context, amount float64, category, description string, occurredAt time.Time) (*domain.Expense, error) {
	return nil, nil
}
func (m *mockExpenseUC) GetMonthlyReport(ctx context.Context, year, month int) (*domain.ExpenseReport, error) {
	return nil, nil
}
func (m *mockExpenseUC) GetDetailMonthlyReport(ctx context.Context, year, month int) ([]*domain.Expense, error) {
	return nil, nil
}
func (m *mockExpenseUC) GetDetailMonthReportByCategory(ctx context.Context, year, month int, category string) ([]*domain.Expense, error) {
	return nil, nil
}
func (m *mockExpenseUC) GetCurrentMonthReport(ctx context.Context) (*domain.ExpenseReport, error) {
	return nil, nil
}
func (m *mockExpenseUC) SendMonthlyReportIfDue(ctx context.Context) error { return nil }
func (m *mockExpenseUC) DeleteExpense(ctx context.Context, id int64) error { return nil }
func (m *mockExpenseUC) GetTodayExpenseReport(ctx context.Context) (*domain.ExpenseReport, error) {
	return m.todayReport, nil
}
func (m *mockExpenseUC) GetTodayExpenses(ctx context.Context) ([]*domain.Expense, error) {
	return m.todayExpenses, nil
}
func (m *mockExpenseUC) GetTodayExpensesByCategory(ctx context.Context, category string) ([]*domain.Expense, error) {
	return nil, nil
}

// Mock Budget Usecase
type mockBudgetUC struct {
	status *domain.BudgetStatus
}

func (m *mockBudgetUC) SetBudget(ctx context.Context, monthlySalary, warningThresholdPercent float64) (*domain.Budget, error) {
	return nil, nil
}
func (m *mockBudgetUC) GetBudgetStatus(ctx context.Context, year, month int) (*domain.BudgetStatus, error) {
	return m.status, nil
}
func (m *mockBudgetUC) CheckExpenseThreshold(ctx context.Context, newExpenseAmount float64) (*domain.BudgetStatus, bool, error) {
	return nil, false, nil
}

// Mock Nutrition Usecase
type mockNutritionUC struct {
	summary *domain.DailyNutritionSummary
}

func (m *mockNutritionUC) LogMealFromImage(ctx context.Context, imageData []byte, mimeType string, notes string) (*domain.FoodLog, *domain.DailyNutritionSummary, error) {
	return nil, nil, nil
}
func (m *mockNutritionUC) LogMealFromText(ctx context.Context, foodName string, calories int, notes string, loggedAt time.Time) (*domain.FoodLog, *domain.DailyNutritionSummary, error) {
	return nil, nil, nil
}
func (m *mockNutritionUC) GetDailySummary(ctx context.Context, date time.Time) (*domain.DailyNutritionSummary, error) {
	return m.summary, nil
}
func (m *mockNutritionUC) DeleteMeal(ctx context.Context, id int64) error {
	return nil
}
func (m *mockNutritionUC) DeleteLatestMeal(ctx context.Context) (*domain.FoodLog, error) {
	return nil, nil
}

// Mock Workout Repository
type mockWorkoutRepo struct {
	workouts []*domain.Workout
}

func (m *mockWorkoutRepo) Create(ctx context.Context, workout *domain.Workout) error { return nil }
func (m *mockWorkoutRepo) GetByID(ctx context.Context, id int64) (*domain.Workout, error) {
	return nil, nil
}
func (m *mockWorkoutRepo) GetByDateRange(ctx context.Context, start, end time.Time) ([]*domain.Workout, error) {
	return m.workouts, nil
}
func (m *mockWorkoutRepo) GetMonthlyStats(ctx context.Context, year, month int) (*domain.WorkoutMonthlyStats, error) {
	return nil, nil
}
func (m *mockWorkoutRepo) GetRecent(ctx context.Context, limit int) ([]*domain.Workout, error) {
	return nil, nil
}
func (m *mockWorkoutRepo) Delete(ctx context.Context, id int64) error { return nil }

// Mock User Profile Usecase
type mockUserProfileUC struct {
	profile *domain.UserProfile
}

func (m *mockUserProfileUC) GetProfile(ctx context.Context) (*domain.UserProfile, error) {
	return m.profile, nil
}
func (m *mockUserProfileUC) UpdatePhysicalMetrics(ctx context.Context, heightCM, weightKG float64, age int, gender, activityLevel string) (*domain.UserProfile, error) {
	return nil, nil
}
func (m *mockUserProfileUC) SetSalary(ctx context.Context, monthlySalary float64) (*domain.UserProfile, error) {
	return nil, nil
}
func (m *mockUserProfileUC) CalculateBMRAndTDEE(profile *domain.UserProfile) (float64, float64) {
	return 0, 0
}

// Mock WhatsApp Sender
type mockWASender struct {
	lastTo  string
	lastMsg string
}

func (m *mockWASender) SendMessage(ctx context.Context, to, message string) error {
	m.lastTo = to
	m.lastMsg = message
	return nil
}

func TestBriefingUsecase_GetDailyBriefing(t *testing.T) {
	tz := time.FixedZone("WIB", 7*3600)
	now := time.Now().In(tz)
	logger := zap.NewNop()

	t.Run("with full data", func(t *testing.T) {
		todayRem := &domain.Reminder{
			ID:          1,
			Title:       "Meeting Tim",
			ScheduledAt: now.Add(2 * time.Hour),
			Status:      "active",
		}
		overdueRem := &domain.Reminder{
			ID:          2,
			Title:       "Bayar Tagihan",
			ScheduledAt: now.Add(-24 * time.Hour),
			Status:      "active",
		}

		reminderRepo := &mockReminderRepo{activeReminders: []*domain.Reminder{todayRem, overdueRem}}
		budgetUC := &mockBudgetUC{
			status: &domain.BudgetStatus{
				MonthlySalary:   5000000,
				CurrentSpent:    2000000,
				Remaining:       3000000,
				SpentPercentage: 40.0,
			},
		}
		userProfileUC := &mockUserProfileUC{
			profile: &domain.UserProfile{
				HeightCM:           175,
				WeightKG:           70,
				DailyCalorieTarget: 2200,
			},
		}
		nutritionUC := &mockNutritionUC{
			summary: &domain.DailyNutritionSummary{
				TargetCalories: 2200,
			},
		}

		waSender := &mockWASender{}
		uc := usecase.NewBriefingUsecase(
			reminderRepo,
			nil,
			budgetUC,
			nutritionUC,
			nil,
			userProfileUC,
			waSender,
			"628123456789@s.whatsapp.net",
			logger,
			tz,
		)

		msg, err := uc.GetDailyBriefing(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(msg, "DAILY BRIEFING") {
			t.Errorf("expected header in message, got: %s", msg)
		}
		if !strings.Contains(msg, "Meeting Tim") {
			t.Errorf("expected today reminder in message, got: %s", msg)
		}
		if !strings.Contains(msg, "Bayar Tagihan") {
			t.Errorf("expected overdue reminder in message, got: %s", msg)
		}
		if !strings.Contains(msg, "3.000.000") {
			t.Errorf("expected remaining budget in message, got: %s", msg)
		}
		if !strings.Contains(msg, "2200 kcal") {
			t.Errorf("expected calorie target in message, got: %s", msg)
		}

		// Test dispatching via WhatsApp
		err = uc.SendDailyBriefing(context.Background())
		if err != nil {
			t.Fatalf("unexpected send error: %v", err)
		}
		if waSender.lastTo != "628123456789@s.whatsapp.net" {
			t.Errorf("expected target phone, got: %s", waSender.lastTo)
		}
	})

	t.Run("with empty data", func(t *testing.T) {
		uc := usecase.NewBriefingUsecase(
			&mockReminderRepo{},
			nil,
			&mockBudgetUC{},
			&mockNutritionUC{},
			nil,
			&mockUserProfileUC{},
			nil,
			"",
			logger,
			tz,
		)

		msg, err := uc.GetDailyBriefing(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(msg, "Tidak ada agenda tersimpan") {
			t.Errorf("expected no agenda message, got: %s", msg)
		}
		if !strings.Contains(msg, "Budget belum diatur") {
			t.Errorf("expected no budget message, got: %s", msg)
		}
	})
}

func TestBriefingUsecase_GetNightlyDebrief(t *testing.T) {
	tz := time.FixedZone("WIB", 7*3600)
	logger := zap.NewNop()

	t.Run("with full evening data", func(t *testing.T) {
		expenseUC := &mockExpenseUC{
			todayReport: &domain.ExpenseReport{
				Total: 150000,
				Breakdown: map[string]float64{
					"Makanan":   100000,
					"Transport": 50000,
				},
			},
			todayExpenses: []*domain.Expense{
				{Amount: 100000, Category: "Makanan"},
				{Amount: 50000, Category: "Transport"},
			},
		}
		budgetUC := &mockBudgetUC{
			status: &domain.BudgetStatus{
				MonthlySalary:   5000000,
				CurrentSpent:    2150000,
				Remaining:       2850000,
				SpentPercentage: 43.0,
			},
		}
		nutritionUC := &mockNutritionUC{
			summary: &domain.DailyNutritionSummary{
				TargetCalories:    2000,
				TotalCalories:     1850,
				RemainingCalories: 150,
				TotalProtein:      80,
				TotalCarbs:        180,
				TotalFat:          50,
			},
		}
		workoutRepo := &mockWorkoutRepo{
			workouts: []*domain.Workout{
				{
					Title:           "Jogging Sore",
					DurationMinutes: 30,
					CaloriesBurned:  250,
					DistanceMeters:  4000,
				},
			},
		}

		waSender := &mockWASender{}
		uc := usecase.NewBriefingUsecase(
			&mockReminderRepo{},
			expenseUC,
			budgetUC,
			nutritionUC,
			workoutRepo,
			nil,
			waSender,
			"628123456789@s.whatsapp.net",
			logger,
			tz,
		)

		msg, err := uc.GetNightlyDebrief(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(msg, "NIGHTLY DEBRIEF") {
			t.Errorf("expected nightly debrief header, got: %s", msg)
		}
		if !strings.Contains(msg, "150.000") {
			t.Errorf("expected expense total, got: %s", msg)
		}
		if !strings.Contains(msg, "Makanan: Rp 100.000") {
			t.Errorf("expected category breakdown, got: %s", msg)
		}
		if !strings.Contains(msg, "1850 / 2000 kcal") {
			t.Errorf("expected nutrition summary, got: %s", msg)
		}
		if !strings.Contains(msg, "Jogging Sore") {
			t.Errorf("expected workout title, got: %s", msg)
		}
		if !strings.Contains(msg, "250 kcal") {
			t.Errorf("expected burned calories, got: %s", msg)
		}

		// Test dispatching via WhatsApp
		err = uc.SendNightlyDebrief(context.Background())
		if err != nil {
			t.Fatalf("unexpected send error: %v", err)
		}
		if waSender.lastTo != "628123456789@s.whatsapp.net" {
			t.Errorf("expected target phone, got: %s", waSender.lastTo)
		}
	})

	t.Run("with zero expenses and rest day", func(t *testing.T) {
		uc := usecase.NewBriefingUsecase(
			&mockReminderRepo{},
			&mockExpenseUC{},
			nil,
			&mockNutritionUC{},
			&mockWorkoutRepo{},
			nil,
			nil,
			"",
			logger,
			tz,
		)

		msg, err := uc.GetNightlyDebrief(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(msg, "*Rp 0*") || !strings.Contains(msg, "Hari hemat") {
			t.Errorf("expected zero expense message, got: %s", msg)
		}
		if !strings.Contains(msg, "Belum ada sesi olahraga hari ini") {
			t.Errorf("expected rest day workout message, got: %s", msg)
		}
	})
}
