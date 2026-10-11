package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
	"go.uber.org/zap"
)

type budgetUsecase struct {
	budgetRepo  domain.BudgetRepository
	expenseRepo domain.ExpenseRepository
	profileUC   domain.UserProfileUsecase
	logger      *zap.Logger
	timezone    *time.Location
}

// NewBudgetUsecase creates a new BudgetUsecase.
func NewBudgetUsecase(
	budgetRepo domain.BudgetRepository,
	expenseRepo domain.ExpenseRepository,
	profileUC domain.UserProfileUsecase,
	logger *zap.Logger,
	timezone *time.Location,
) domain.BudgetUsecase {
	return &budgetUsecase{
		budgetRepo:  budgetRepo,
		expenseRepo: expenseRepo,
		profileUC:   profileUC,
		logger:      logger,
		timezone:    timezone,
	}
}

func (u *budgetUsecase) SetBudget(ctx context.Context, monthlySalary, warningThresholdPercent float64) (*domain.Budget, error) {
	if warningThresholdPercent <= 0 {
		warningThresholdPercent = domain.DefaultWarningThresholdPercent
	}

	budget := &domain.Budget{
		MonthlySalary:           monthlySalary,
		WarningThresholdPercent: warningThresholdPercent,
	}

	if err := u.budgetRepo.Save(ctx, budget); err != nil {
		return nil, fmt.Errorf("budget usecase set: %w", err)
	}

	// Sinkronkan juga ke UserProfile jika profileUC tersedia
	if u.profileUC != nil {
		_, _ = u.profileUC.SetSalary(ctx, monthlySalary)
	}

	u.logger.Info("budget settings saved",
		zap.Float64("salary", monthlySalary),
		zap.Float64("threshold_percent", warningThresholdPercent),
	)
	return budget, nil
}

func (u *budgetUsecase) GetBudgetStatus(ctx context.Context, year, month int) (*domain.BudgetStatus, error) {
	budget, err := u.budgetRepo.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("get budget config: %w", err)
	}

	salary := 0.0
	thresholdPercent := domain.DefaultWarningThresholdPercent
	if budget != nil && budget.MonthlySalary > 0 {
		salary = budget.MonthlySalary
		thresholdPercent = budget.WarningThresholdPercent
	} else if u.profileUC != nil {
		profile, _ := u.profileUC.GetProfile(ctx)
		if profile != nil && profile.MonthlySalary > 0 {
			salary = profile.MonthlySalary
		}
	}

	report, err := u.expenseRepo.GetMonthlyReport(ctx, year, month)
	if err != nil {
		return nil, fmt.Errorf("get monthly expense report for budget: %w", err)
	}

	totalSpent := 0.0
	if report != nil {
		totalSpent = report.Total
	}

	remaining := salary - totalSpent
	spentPct := 0.0
	if salary > 0 {
		spentPct = (totalSpent / salary) * 100.0
	}

	isExceeded := salary > 0 && totalSpent > salary
	isThresholdReached := salary > 0 && spentPct >= thresholdPercent

	var warningMsg string
	if isExceeded {
		warningMsg = fmt.Sprintf("🚨 *PERINGATAN OVERBUDGET:* Pengeluaran bulan ini (Rp %s) telah melebihi gaji bulananmu (Rp %s) sebesar Rp %s! Rem pengeluaran sekarang wir 🛑",
			formatRupiah(totalSpent),
			formatRupiah(salary),
			formatRupiah(totalSpent-salary),
		)
	} else if isThresholdReached {
		warningMsg = fmt.Sprintf("⚠️ *PERINGATAN BUDGET:* Pengeluaran telah mencapai %.1f%% dari batas gaji bulanan (Rp %s dari Rp %s). Sisa kuota: Rp %s.",
			spentPct,
			formatRupiah(totalSpent),
			formatRupiah(salary),
			formatRupiah(remaining),
		)
	}

	return &domain.BudgetStatus{
		Period:                  fmt.Sprintf("%d-%02d", year, month),
		MonthlySalary:           salary,
		CurrentSpent:            totalSpent,
		Remaining:               remaining,
		SpentPercentage:         spentPct,
		WarningThresholdPercent: thresholdPercent,
		IsThresholdReached:      isThresholdReached,
		IsExceeded:              isExceeded,
		WarningMessage:          warningMsg,
	}, nil
}

func (u *budgetUsecase) CheckExpenseThreshold(ctx context.Context, newExpenseAmount float64) (status *domain.BudgetStatus, alertTriggered bool, err error) {
	now := time.Now().In(u.timezone)
	status, err = u.GetBudgetStatus(ctx, now.Year(), int(now.Month()))
	if err != nil {
		return nil, false, err
	}

	if status.MonthlySalary <= 0 {
		// Belum setting gaji bulanan, tidak trigger alert
		return status, false, nil
	}

	// Periksa apakah penambahan biaya ini memicu threshold atau overbudget
	if status.IsExceeded || status.IsThresholdReached {
		return status, true, nil
	}

	return status, false, nil
}

// FormatBudgetStatus formats status into a readable string for WA.
func FormatBudgetStatus(status *domain.BudgetStatus) string {
	if status == nil || status.MonthlySalary <= 0 {
		return "Gaji/budget bulanan belum disetel. Ketik misal: *set gaji 10jt* untuk menyetel batas anggaran."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("💳 *Status Anggaran Bulan (%s)*\n\n", status.Period))
	sb.WriteString(fmt.Sprintf("• Gaji / Batas: *Rp %s*\n", formatRupiah(status.MonthlySalary)))
	sb.WriteString(fmt.Sprintf("• Terpakai: *Rp %s* (%.1f%%)\n", formatRupiah(status.CurrentSpent), status.SpentPercentage))
	if status.Remaining >= 0 {
		sb.WriteString(fmt.Sprintf("• Sisa Saldo: *Rp %s*\n", formatRupiah(status.Remaining)))
	} else {
		sb.WriteString(fmt.Sprintf("• Melebihi Anggaran: *Rp %s* 🚨\n", formatRupiah(-status.Remaining)))
	}
	sb.WriteString(fmt.Sprintf("• Threshold Peringatan: %.0f%%\n", status.WarningThresholdPercent))

	if status.WarningMessage != "" {
		sb.WriteString(fmt.Sprintf("\n%s\n", status.WarningMessage))
	}
	return sb.String()
}
