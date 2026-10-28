package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
	"go.uber.org/zap"
)

type expenseUsecase struct {
	repo     domain.ExpenseRepository
	waSender domain.WhatsAppSender
	waPhone  string
	logger   *zap.Logger
	timezone *time.Location
}

// NewExpenseUsecase creates a new ExpenseUsecase.
func NewExpenseUsecase(
	repo domain.ExpenseRepository,
	waSender domain.WhatsAppSender,
	waPhone string,
	logger *zap.Logger,
	timezone *time.Location,
) domain.ExpenseUsecase {
	return &expenseUsecase{
		repo:     repo,
		waSender: waSender,
		waPhone:  waPhone,
		logger:   logger,
		timezone: timezone,
	}
}

// RecordExpense saves a new expense entry.
func (u *expenseUsecase) RecordExpense(ctx context.Context, amount float64, category, description string, occurredAt time.Time) (*domain.Expense, error) {
	if category == "" {
		category = domain.CategoryOther
	}
	expense := &domain.Expense{
		Amount:      amount,
		Category:    category,
		Description: description,
		OccurredAt:  occurredAt,
		CreatedAt:   time.Now(),
	}
	if err := u.repo.Create(ctx, expense); err != nil {
		return nil, fmt.Errorf("record expense: %w", err)
	}
	u.logger.Info("expense recorded", zap.Int64("id", expense.ID), zap.Float64("amount", amount), zap.String("category", category))
	return expense, nil
}

// GetMonthlyReport returns aggregated expense data for a specific month.
func (u *expenseUsecase) GetMonthlyReport(ctx context.Context, year, month int) (*domain.ExpenseReport, error) {
	return u.repo.GetMonthlyReport(ctx, year, month)
}

// GetCurrentMonthReport returns the report for the current calendar month.
func (u *expenseUsecase) GetCurrentMonthReport(ctx context.Context) (*domain.ExpenseReport, error) {
	now := time.Now().In(u.timezone)
	return u.repo.GetMonthlyReport(ctx, now.Year(), int(now.Month()))
}

// DeleteExpense removes an expense by its ID.
func (u *expenseUsecase) DeleteExpense(ctx context.Context, id int64) error {
	if err := u.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete expense id %d: %w", id, err)
	}
	u.logger.Info("expense deleted", zap.Int64("id", id))
	return nil
}

// SendMonthlyReportIfDue sends the monthly expense report via WhatsApp
// if today is the last day of the month.
func (u *expenseUsecase) SendMonthlyReportIfDue(ctx context.Context) error {
	now := time.Now().In(u.timezone)
	// Check if today is the last day of the month
	tomorrow := now.AddDate(0, 0, 1)
	if tomorrow.Day() != 1 {
		return nil // Not the last day of the month
	}

	report, err := u.repo.GetMonthlyReport(ctx, now.Year(), int(now.Month()))
	if err != nil {
		return fmt.Errorf("send monthly report: fetch report: %w", err)
	}

	msg := FormatExpenseReport(report)
	if err := u.waSender.SendMessage(ctx, u.waPhone, msg); err != nil {
		return fmt.Errorf("send monthly report: wa send: %w", err)
	}
	u.logger.Info("monthly expense report sent", zap.String("period", report.Period))
	return nil
}

// FormatExpenseReport formats an ExpenseReport into a readable WA message.
func FormatExpenseReport(report *domain.ExpenseReport) string {
	if report == nil {
		return "Tidak ada data pengeluaran untuk periode ini."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Rekap Pengeluaran wir 😹 %s*\n\n", report.Period))
	sb.WriteString(fmt.Sprintf("Total: *Rp %s*\n\n", formatRupiah(report.Total)))

	if len(report.Breakdown) > 0 {
		sb.WriteString("📊 *Rincian per kategori:*\n")
		for cat, amount := range report.Breakdown {
			pct := 0.0
			if report.Total > 0 {
				pct = amount / report.Total * 100
			}
			sb.WriteString(fmt.Sprintf("• %s: Rp %s (%.1f%%)\n", cat, formatRupiah(amount), pct))
		}
	}

	sb.WriteString(fmt.Sprintf("\n_Periode: %s s/d %s_",
		report.StartDate.Format("02 Jan"),
		report.EndDate.Format("02 Jan 2006"),
	))
	return sb.String()
}

// GetDetailMonthlyReport returns detailed expense data for a specific month.
func (u *expenseUsecase) GetDetailMonthlyReport(ctx context.Context, year, month int) ([]*domain.Expense, error) {
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, u.timezone)
	end := start.AddDate(0, 1, 0).Add(-time.Second)
	return u.repo.GetByDateRange(ctx, start, end)
}

// GetDetailMonthReportByCategory returns detailed expense data for a specific month filtered by category.
func (u *expenseUsecase) GetDetailMonthReportByCategory(ctx context.Context, year, month int, category string) ([]*domain.Expense, error) {
	expenses, err := u.GetDetailMonthlyReport(ctx, year, month)
	if err != nil {
		return nil, err
	}

	targetCat := NormalizeExpenseCategory(category)
	var filtered []*domain.Expense
	for _, exp := range expenses {
		if NormalizeExpenseCategory(exp.Category) == targetCat || strings.EqualFold(exp.Category, category) {
			filtered = append(filtered, exp)
		}
	}
	return filtered, nil
}

// GetTodayExpenses returns detailed expense data for today.
func (u *expenseUsecase) GetTodayExpenses(ctx context.Context) ([]*domain.Expense, error) {
	now := time.Now().In(u.timezone)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, u.timezone)
	end := start.AddDate(0, 0, 1).Add(-time.Second)
	return u.repo.GetByDateRange(ctx, start, end)
}

// GetTodayExpensesByCategory returns detailed expense data for today filtered by category.
func (u *expenseUsecase) GetTodayExpensesByCategory(ctx context.Context, category string) ([]*domain.Expense, error) {
	expenses, err := u.GetTodayExpenses(ctx)
	if err != nil {
		return nil, err
	}

	targetCat := NormalizeExpenseCategory(category)
	var filtered []*domain.Expense
	for _, exp := range expenses {
		if NormalizeExpenseCategory(exp.Category) == targetCat || strings.EqualFold(exp.Category, category) {
			filtered = append(filtered, exp)
		}
	}
	return filtered, nil
}

// GetTodayExpenseReport returns aggregated expense data for the current day.
func (u *expenseUsecase) GetTodayExpenseReport(ctx context.Context) (*domain.ExpenseReport, error) {
	now := time.Now().In(u.timezone)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, u.timezone)
	end := start.AddDate(0, 0, 1).Add(-time.Second)
	expenses, err := u.GetTodayExpenses(ctx)
	if err != nil {
		return nil, fmt.Errorf("error get today expense report: %w", err)
	}
	report := &domain.ExpenseReport{
		Period:    "Hari Ini",
		StartDate: start,
		EndDate:   end,
		Total:     0,
		Breakdown: make(map[string]float64),
	}
	for _, exp := range expenses {
		report.Total += exp.Amount
		report.Breakdown[exp.Category] += exp.Amount
	}
	return report, nil
}

// NormalizeExpenseCategory maps category synonyms or variations to canonical domain categories.
func NormalizeExpenseCategory(cat string) string {
	c := strings.ToLower(strings.TrimSpace(cat))
	switch c {
	case "makanan", "makan", "food", "kuliner":
		return domain.CategoryFood
	case "transportasi", "transport", "ojol", "bensin", "parkir":
		return domain.CategoryTransport
	case "hiburan", "entertainment", "game", "nonton", "bioskop":
		return domain.CategoryEntertainment
	case "kesehatan", "health", "obat", "dokter", "medis":
		return domain.CategoryHealth
	case "belanja", "shopping", "groceries":
		return domain.CategoryShopping
	case "tagihan", "bill", "bills", "listrik", "air", "wifi", "pulsa":
		return domain.CategoryBill
	case "amal", "charity", "sedekah", "infaq", "donasi":
		return domain.CategoryCharity
	case "lainnya", "other", "others":
		return domain.CategoryOther
	default:
		return c
	}
}

// FormatDetailExpenseList formats a list of expenses into a detailed WA message.
func FormatDetailExpenseList(expenses []*domain.Expense, period string, category string) string {
	if len(expenses) == 0 {
		if category != "" {
			return fmt.Sprintf("Belum ada catatan pengeluaran untuk kategori *%s* pada periode %s nih wir! 💸", category, period)
		}
		return fmt.Sprintf("Belum ada catatan pengeluaran pada periode %s nih wir! 💸", period)
	}

	var sb strings.Builder
	if category != "" {
		sb.WriteString(fmt.Sprintf("📋 *Detail Pengeluaran %s (%s)*\n\n", strings.Title(category), period))
	} else {
		sb.WriteString(fmt.Sprintf("📋 *Daftar Seluruh Pengeluaran (%s)*\n\n", period))
	}

	var total float64
	for i, exp := range expenses {
		total += exp.Amount
		dateStr := exp.OccurredAt.Format("02 Jan 15:04")
		sb.WriteString(fmt.Sprintf("%d. [%s] *%s*\n   • Kategori: %s\n   • Jumlah: Rp %s (ID: %d)\n",
			i+1,
			dateStr,
			exp.Description,
			exp.Category,
			formatRupiah(exp.Amount),
			exp.ID,
		))
	}

	sb.WriteString(fmt.Sprintf("\n💰 *Total: Rp %s* (%d transaksi)", formatRupiah(total), len(expenses)))
	return sb.String()
}

// FormatExpenseConfirmation returns a short confirmation message after recording.
func FormatExpenseConfirmation(expense *domain.Expense) string {
	return fmt.Sprintf("✅ Pengeluaran dicatat!\n• %s\n• Kategori: %s\n• Jumlah: Rp %s\n Jangan boros-boros loh ya😹 cari duid susah",
		expense.Description,
		expense.Category,
		formatRupiah(expense.Amount),
	)
}

// formatRupiah formats a float as Indonesian Rupiah string (e.g. 1.500.000).
func formatRupiah(amount float64) string {
	s := fmt.Sprintf("%.0f", amount)
	n := len(s)
	if n <= 3 {
		return s
	}
	var result strings.Builder
	for i, ch := range s {
		if i > 0 && (n-i)%3 == 0 {
			result.WriteRune('.')
		}
		result.WriteRune(ch)
	}
	return result.String()
}
