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

type mockExpenseRepo struct {
	expenses []*domain.Expense
}

func (m *mockExpenseRepo) Create(ctx context.Context, expense *domain.Expense) error {
	return nil
}

func (m *mockExpenseRepo) GetByID(ctx context.Context, id int64) (*domain.Expense, error) {
	return nil, nil
}

func (m *mockExpenseRepo) GetByDateRange(ctx context.Context, start, end time.Time) ([]*domain.Expense, error) {
	var result []*domain.Expense
	for _, e := range m.expenses {
		if (e.OccurredAt.After(start) || e.OccurredAt.Equal(start)) && (e.OccurredAt.Before(end) || e.OccurredAt.Equal(end)) {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockExpenseRepo) GetMonthlyReport(ctx context.Context, year int, month int) (*domain.ExpenseReport, error) {
	return nil, nil
}

func (m *mockExpenseRepo) Delete(ctx context.Context, id int64) error {
	return nil
}

func TestExpenseDetailReports(t *testing.T) {
	loc := time.FixedZone("WIB", 7*3600)
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, loc)

	mockRepo := &mockExpenseRepo{
		expenses: []*domain.Expense{
			{ID: 1, Amount: 25000, Category: "makan", Description: "Nasi Goreng", OccurredAt: now},
			{ID: 2, Amount: 15000, Category: "transport", Description: "Ojek Online", OccurredAt: now.Add(time.Hour)},
			{ID: 3, Amount: 50000, Category: "amal", Description: "Infaq Jumat", OccurredAt: now.Add(2 * time.Hour)},
		},
	}

	uc := usecase.NewExpenseUsecase(mockRepo, nil, "", zap.NewNop(), loc)

	t.Run("GetDetailMonthlyReport - All expenses", func(t *testing.T) {
		res, err := uc.GetDetailMonthlyReport(context.Background(), 2026, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 3 {
			t.Fatalf("expected 3 expenses, got %d", len(res))
		}
	})

	t.Run("GetDetailMonthReportByCategory - Filter makan", func(t *testing.T) {
		res, err := uc.GetDetailMonthReportByCategory(context.Background(), 2026, 10, "makan")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 || res[0].Description != "Nasi Goreng" {
			t.Fatalf("expected Nasi Goreng, got %+v", res)
		}
	})

	t.Run("GetDetailMonthReportByCategory - Synonym makanan", func(t *testing.T) {
		res, err := uc.GetDetailMonthReportByCategory(context.Background(), 2026, 10, "makanan")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 || res[0].Category != "makan" {
			t.Fatalf("expected 1 makan expense, got %+v", res)
		}
	})

	t.Run("GetDetailMonthReportByCategory - Charity / Amal", func(t *testing.T) {
		res, err := uc.GetDetailMonthReportByCategory(context.Background(), 2026, 10, "sedekah")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 || res[0].Category != "amal" {
			t.Fatalf("expected 1 amal expense, got %+v", res)
		}
	})

	t.Run("ParseExpenseTargetPeriod - bulan lalu", func(t *testing.T) {
		y, m, p := usecase.ParseExpenseTargetPeriod("list pengeluaran bulan lalu", now)
		if y != 2026 || m != 9 || p != "September 2026" {
			t.Fatalf("expected 2026-09 'September 2026', got %d-%d '%s'", y, m, p)
		}
	})

	t.Run("ParseExpenseTargetPeriod - bulan kemarin", func(t *testing.T) {
		y, m, p := usecase.ParseExpenseTargetPeriod("rekap pengeluaran makan bulan kemarin", now)
		if y != 2026 || m != 9 || p != "September 2026" {
			t.Fatalf("expected 2026-09 'September 2026', got %d-%d '%s'", y, m, p)
		}
	})

	t.Run("ParseExpenseTargetPeriod - explicit month", func(t *testing.T) {
		y, m, p := usecase.ParseExpenseTargetPeriod("detail pengeluaran agustus", now)
		if y != 2026 || m != 8 || p != "Agustus 2026" {
			t.Fatalf("expected 2026-08 'Agustus 2026', got %d-%d '%s'", y, m, p)
		}
	})

	t.Run("ParseExpenseTargetPeriod - explicit month with year", func(t *testing.T) {
		y, m, p := usecase.ParseExpenseTargetPeriod("laporan pengeluaran mei 2025", now)
		if y != 2025 || m != 5 || p != "Mei 2025" {
			t.Fatalf("expected 2025-05 'Mei 2025', got %d-%d '%s'", y, m, p)
		}
	})

	t.Run("ParseExpenseTargetPeriod - current month default", func(t *testing.T) {
		y, m, p := usecase.ParseExpenseTargetPeriod("list pengeluaran", now)
		if y != 2026 || m != 10 || p != "Oktober 2026" {
			t.Fatalf("expected 2026-10 'Oktober 2026', got %d-%d '%s'", y, m, p)
		}
	})

	t.Run("ParseExpenseTargetPeriod - hari ini", func(t *testing.T) {
		y, m, p := usecase.ParseExpenseTargetPeriod("list pengeluaran hari ini", now)
		if y != 2026 || m != 10 || p != "Hari ini (15 Oktober 2026)" {
			t.Fatalf("expected 2026-10 'Hari ini (15 Oktober 2026)', got %d-%d '%s'", y, m, p)
		}
	})

	t.Run("GetTodayExpenses and GetTodayExpenseReport", func(t *testing.T) {
		todayExp := &domain.Expense{
			ID:          10,
			Amount:      35000,
			Category:    "makan",
			Description: "Ayam Geprek Hari Ini",
			OccurredAt:  time.Now().In(loc),
		}
		mockRepo.expenses = append(mockRepo.expenses, todayExp)

		todayList, err := uc.GetTodayExpenses(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(todayList) == 0 {
			t.Fatalf("expected at least 1 today expense, got 0")
		}

		todayMakan, err := uc.GetTodayExpensesByCategory(context.Background(), "makan")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(todayMakan) == 0 {
			t.Fatalf("expected at least 1 today makan expense, got 0")
		}

		rep, err := uc.GetTodayExpenseReport(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rep.Period != "Hari Ini" || rep.Total <= 0 {
			t.Fatalf("expected today report with positive total, got %+v", rep)
		}

		formatted := usecase.FormatDetailExpenseList(todayList, "Hari ini (15 Okt 2026)", "")
		if formatted == "" || !strings.Contains(formatted, "Ayam Geprek Hari Ini") {
			t.Fatalf("formatted output missing item description: %s", formatted)
		}
	})
}
