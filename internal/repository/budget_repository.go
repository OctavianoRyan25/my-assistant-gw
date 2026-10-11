package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
)

type budgetRepository struct {
	db *sql.DB
}

// NewBudgetRepository creates a new BudgetRepository.
func NewBudgetRepository(db *sql.DB) domain.BudgetRepository {
	return &budgetRepository{db: db}
}

func (r *budgetRepository) Get(ctx context.Context) (*domain.Budget, error) {
	query := `
		SELECT id, monthly_salary, warning_threshold_percent, created_at, updated_at
		FROM budget_settings
		ORDER BY id ASC
		LIMIT 1
	`
	row := r.db.QueryRowContext(ctx, query)

	var b domain.Budget
	err := row.Scan(
		&b.ID,
		&b.MonthlySalary,
		&b.WarningThresholdPercent,
		&b.CreatedAt,
		&b.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("budget repository get: %w", err)
	}
	return &b, nil
}

func (r *budgetRepository) Save(ctx context.Context, budget *domain.Budget) error {
	existing, err := r.Get(ctx)
	if err != nil {
		return err
	}

	now := time.Now()
	if existing == nil {
		query := `
			INSERT INTO budget_settings (monthly_salary, warning_threshold_percent, created_at, updated_at)
			VALUES (?, ?, ?, ?)
		`
		result, err := r.db.ExecContext(ctx, query,
			budget.MonthlySalary,
			budget.WarningThresholdPercent,
			now,
			now,
		)
		if err != nil {
			return fmt.Errorf("budget repository insert: %w", err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("budget repository get insert id: %w", err)
		}
		budget.ID = id
		budget.CreatedAt = now
		budget.UpdatedAt = now
		return nil
	}

	query := `
		UPDATE budget_settings
		SET monthly_salary = ?, warning_threshold_percent = ?, updated_at = ?
		WHERE id = ?
	`
	_, err = r.db.ExecContext(ctx, query,
		budget.MonthlySalary,
		budget.WarningThresholdPercent,
		now,
		existing.ID,
	)
	if err != nil {
		return fmt.Errorf("budget repository update: %w", err)
	}
	budget.ID = existing.ID
	budget.UpdatedAt = now
	return nil
}
