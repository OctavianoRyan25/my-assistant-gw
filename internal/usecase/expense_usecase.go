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
