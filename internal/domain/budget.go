package domain

import (
	"context"
	"time"
)

// Default warning threshold (80% of monthly salary)
const DefaultWarningThresholdPercent = 80.0

// Budget defines the monthly budget and warning threshold configuration.
type Budget struct {
	ID                      int64     `json:"id"`
	MonthlySalary           float64   `json:"monthly_salary"`            // Gaji atau batas pengeluaran per bulan
	WarningThresholdPercent float64   `json:"warning_threshold_percent"` // Persentase ambang batas peringatan (e.g. 80.0)
	Period                  string    `json:"period"`                    // e.g. "Oktober 2026"
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

// BudgetStatus represents the evaluation of current expenses against the budget threshold.
type BudgetStatus struct {
	Period                  string  `json:"period"`                    // e.g. "Oktober 2026"
	MonthlySalary           float64 `json:"monthly_salary"`            // Gaji per bulan
	CurrentSpent            float64 `json:"current_spent"`             // Total pengeluaran bulan berjalan
	Remaining               float64 `json:"remaining"`                 // Sisa anggaran (MonthlySalary - CurrentSpent)
	SpentPercentage         float64 `json:"spent_percentage"`          // Persentase yang telah terpakai
	WarningThresholdPercent float64 `json:"warning_threshold_percent"` // Ambang batas peringatan
	IsThresholdReached      bool    `json:"is_threshold_reached"`      // True jika SpentPercentage >= WarningThresholdPercent
	IsExceeded              bool    `json:"is_exceeded"`               // True jika CurrentSpent > MonthlySalary
	WarningMessage          string  `json:"warning_message,omitempty"` // Pesan peringatan jika threshold/budget terlampaui
}

// BudgetRepository defines persistence operations for budget settings.
type BudgetRepository interface {
	Get(ctx context.Context, period string) (*Budget, error)
	GetLatest(ctx context.Context) (*Budget, error)
	Save(ctx context.Context, period string, budget *Budget) error
}

// BudgetUsecase defines business logic for budget monitoring and salary threshold alerts.
type BudgetUsecase interface {
	SetBudget(ctx context.Context, monthlySalary, warningThresholdPercent float64) (*Budget, error)
	GetBudgetStatus(ctx context.Context, year, month int) (*BudgetStatus, error)
	CheckExpenseThreshold(ctx context.Context, newExpenseAmount float64) (status *BudgetStatus, alertTriggered bool, err error)
}
