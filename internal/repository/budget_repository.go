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

// NewBudgetRepository creates a new BudgetRepository and ensures table exists.
func NewBudgetRepository(db *sql.DB) domain.BudgetRepository {
	repo := &budgetRepository{db: db}
	repo.ensureTableExists()
	return repo
}

func (r *budgetRepository) ensureTableExists() {
	query := `
		CREATE TABLE IF NOT EXISTS budget_settings (
			id                        BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			monthly_salary            DECIMAL(15, 2)  NOT NULL DEFAULT 0,
			warning_threshold_percent DECIMAL(5, 2)   NOT NULL DEFAULT 80.00,
			period                    VARCHAR(7)      NOT NULL DEFAULT '2026-01',
			created_at                DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at                DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
	`
	_, _ = r.db.Exec(query)
}

func (r *budgetRepository) Get(ctx context.Context, period string) (*domain.Budget, error) {
	query := `
		SELECT id, monthly_salary, warning_threshold_percent, period, created_at, updated_at
		FROM budget_settings
		WHERE period = ?
		LIMIT 1
	`
	row := r.db.QueryRowContext(ctx, query, period)

	var b domain.Budget
	err := row.Scan(
		&b.ID,
		&b.MonthlySalary,
		&b.WarningThresholdPercent,
		&b.Period,
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

func (r *budgetRepository) GetLatest(ctx context.Context) (*domain.Budget, error) {
	query := `
		SELECT id, monthly_salary, warning_threshold_percent, period, created_at, updated_at
		FROM budget_settings
		ORDER BY period DESC, updated_at DESC
		LIMIT 1
	`
	row := r.db.QueryRowContext(ctx, query)

	var b domain.Budget
	err := row.Scan(
		&b.ID,
		&b.MonthlySalary,
		&b.WarningThresholdPercent,
		&b.Period,
		&b.CreatedAt,
		&b.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("budget repository get latest: %w", err)
	}
	return &b, nil
}

func (r *budgetRepository) Save(ctx context.Context, period string, budget *domain.Budget) error {
	existing, err := r.Get(ctx, period)
	if err != nil {
		return err
	}

	now := time.Now()
	if existing == nil {
		query := `
			INSERT INTO budget_settings (monthly_salary, warning_threshold_percent, period, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`
		result, err := r.db.ExecContext(ctx, query,
			budget.MonthlySalary,
			budget.WarningThresholdPercent,
			period,
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
		SET monthly_salary = ?, warning_threshold_percent = ?, period = ?, updated_at = ?
		WHERE period = ?
	`
	_, err = r.db.ExecContext(ctx, query,
		budget.MonthlySalary,
		budget.WarningThresholdPercent,
		period,
		now,
		existing.Period,
	)
	if err != nil {
		return fmt.Errorf("budget repository update: %w", err)
	}
	budget.ID = existing.ID
	budget.UpdatedAt = now
	return nil
}
