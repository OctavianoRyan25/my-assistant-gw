package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
	"github.com/OctavianoRyan25/my-assistant-gw/internal/usecase"
	"go.uber.org/zap"
)

type mockBudgetRepo struct {
	budgets map[string]*domain.Budget
	latest  *domain.Budget
}

func (m *mockBudgetRepo) Get(ctx context.Context, period string) (*domain.Budget, error) {
	if b, ok := m.budgets[period]; ok {
		return b, nil
	}
	return nil, nil
}

func (m *mockBudgetRepo) GetLatest(ctx context.Context) (*domain.Budget, error) {
	return m.latest, nil
}

func (m *mockBudgetRepo) Save(ctx context.Context, period string, budget *domain.Budget) error {
	if m.budgets == nil {
		m.budgets = make(map[string]*domain.Budget)
	}
	budget.Period = period
	m.budgets[period] = budget
	m.latest = budget
	return nil
}

type mockBudgetExpenseRepo struct {
	total float64
}

func (m *mockBudgetExpenseRepo) Create(ctx context.Context, expense *domain.Expense) error {
	return nil
}
func (m *mockBudgetExpenseRepo) GetByID(ctx context.Context, id int64) (*domain.Expense, error) {
	return nil, nil
}
func (m *mockBudgetExpenseRepo) GetByDateRange(ctx context.Context, start, end time.Time) ([]*domain.Expense, error) {
	return nil, nil
}
func (m *mockBudgetExpenseRepo) GetMonthlyReport(ctx context.Context, year int, month int) (*domain.ExpenseReport, error) {
	return &domain.ExpenseReport{
		Period: "Oktober 2026",
		Total:  m.total,
	}, nil
}
func (m *mockBudgetExpenseRepo) Delete(ctx context.Context, id int64) error {
	return nil
}

func TestBudgetStatusAndFormatting(t *testing.T) {
	loc := time.FixedZone("WIB", 7*3600)
	ctx := context.Background()

	bRepo := &mockBudgetRepo{}
	eRepo := &mockBudgetExpenseRepo{total: 4500000} // Spent 4.5jt

	uc := usecase.NewBudgetUsecase(bRepo, eRepo, nil, zap.NewNop(), loc)

	// 1. Set budget 5jt, threshold 80%
	_, err := uc.SetBudget(ctx, 5000000, 80)
	if err != nil {
		t.Fatalf("unexpected error setting budget: %v", err)
	}

	// 2. Cek status: spent 4.5jt / 5jt = 90% (Threshold reached!)
	status, err := uc.GetBudgetStatus(ctx, 2026, 10)
	if err != nil {
		t.Fatalf("unexpected error getting status: %v", err)
	}

	if status.MonthlySalary != 5000000 {
		t.Errorf("expected salary 5000000, got %v", status.MonthlySalary)
	}
	if status.Remaining != 500000 {
		t.Errorf("expected remaining 500000, got %v", status.Remaining)
	}
	if !status.IsThresholdReached {
		t.Errorf("expected threshold to be reached (90%% >= 80%%)")
	}

	formatted := usecase.FormatBudgetStatus(status)
	if formatted == "" {
		t.Errorf("expected formatted output not to be empty")
	}

	// 3. Test check threshold trigger
	_, alertTriggered, err := uc.CheckExpenseThreshold(ctx, 100000)
	if err != nil || !alertTriggered {
		t.Errorf("expected alert to be triggered, got %v, err: %v", alertTriggered, err)
	}
}

func TestParseBudgetSettingInput(t *testing.T) {
	tests := []struct {
		input        string
		wantSalary   float64
		wantThresh   float64
		hasSalary    bool
		hasThreshold bool
	}{
		{
			input:        "set budget 5jt",
			wantSalary:   5000000,
			hasSalary:    true,
			hasThreshold: false,
		},
		{
			input:        "gaji bulan ini 8.000.000 threshold 85%",
			wantSalary:   8000000,
			wantThresh:   85,
			hasSalary:    true,
			hasThreshold: true,
		},
		{
			input:        "atur budget 10jt ambang batas 75",
			wantSalary:   10000000,
			wantThresh:   75,
			hasSalary:    true,
			hasThreshold: true,
		},
		{
			input:        "ubah threshold jadi 90%",
			hasSalary:    false,
			wantThresh:   90,
			hasThreshold: true,
		},
		{
			input:        "budget bulan ini",
			hasSalary:    false,
			hasThreshold: false,
		},
		{
			input:        "sisa uang bulan ini berapa",
			hasSalary:    false,
			hasThreshold: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			s, th, hasS, hasTh := usecase.ParseBudgetSettingInput(tt.input)
			if hasS != tt.hasSalary {
				t.Errorf("hasSalary: expected %v, got %v", tt.hasSalary, hasS)
			}
			if tt.hasSalary && s != tt.wantSalary {
				t.Errorf("salary: expected %v, got %v", tt.wantSalary, s)
			}
			if hasTh != tt.hasThreshold {
				t.Errorf("hasThreshold: expected %v, got %v", tt.hasThreshold, hasTh)
			}
			if tt.hasThreshold && th != tt.wantThresh {
				t.Errorf("threshold: expected %v, got %v", tt.wantThresh, th)
			}
		})
	}
}
