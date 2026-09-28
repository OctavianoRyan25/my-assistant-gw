package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
)

type expenseRepository struct {
	db *sql.DB
}

// NewExpenseRepository creates a new MySQL-backed ExpenseRepository.
func NewExpenseRepository(db *sql.DB) domain.ExpenseRepository {
	return &expenseRepository{db: db}
}

func (r *expenseRepository) Create(ctx context.Context, expense *domain.Expense) error {
	query := `
		INSERT INTO expenses (amount, category, description, occurred_at, created_at)
		VALUES (?, ?, ?, ?, ?)
	`
	result, err := r.db.ExecContext(ctx, query,
		expense.Amount,
		expense.Category,
		expense.Description,
		expense.OccurredAt,
		expense.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("expense repository create: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("expense repository get last insert id: %w", err)
	}
	expense.ID = id
	return nil
}

func (r *expenseRepository) GetByID(ctx context.Context, id int64) (*domain.Expense, error) {
	query := `SELECT id, amount, category, description, occurred_at, created_at FROM expenses WHERE id = ?`
	row := r.db.QueryRowContext(ctx, query, id)
	var exp domain.Expense
	err := row.Scan(&exp.ID, &exp.Amount, &exp.Category, &exp.Description, &exp.OccurredAt, &exp.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("expense repository get by id: %w", err)
	}
	return &exp, nil
}

func (r *expenseRepository) GetByDateRange(ctx context.Context, start, end time.Time) ([]*domain.Expense, error) {
	query := `
		SELECT id, amount, category, description, occurred_at, created_at
		FROM expenses
		WHERE occurred_at >= ? AND occurred_at <= ?
		ORDER BY occurred_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, start, end)
	if err != nil {
		return nil, fmt.Errorf("expense repository get by date range: %w", err)
	}
	defer rows.Close()

	var expenses []*domain.Expense
	for rows.Next() {
		var exp domain.Expense
		if err := rows.Scan(&exp.ID, &exp.Amount, &exp.Category, &exp.Description, &exp.OccurredAt, &exp.CreatedAt); err != nil {
			return nil, fmt.Errorf("expense repository scan row: %w", err)
		}
		expenses = append(expenses, &exp)
	}
	return expenses, rows.Err()
}

func (r *expenseRepository) GetMonthlyReport(ctx context.Context, year int, month int) (*domain.ExpenseReport, error) {
	// Calculate date range for the given month
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	end := start.AddDate(0, 1, 0).Add(-time.Second) // last second of the month

	// Get total
	var total float64
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM expenses WHERE occurred_at >= ? AND occurred_at <= ?`,
		start, end,
	).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("expense repository get monthly total: %w", err)
	}

	// Get breakdown by category
	rows, err := r.db.QueryContext(ctx,
		`SELECT category, COALESCE(SUM(amount), 0) as total
		 FROM expenses
		 WHERE occurred_at >= ? AND occurred_at <= ?
		 GROUP BY category
		 ORDER BY total DESC`,
		start, end,
	)
	if err != nil {
		return nil, fmt.Errorf("expense repository get category breakdown: %w", err)
	}
	defer rows.Close()

	breakdown := make(map[string]float64)
	for rows.Next() {
		var category string
		var catTotal float64
		if err := rows.Scan(&category, &catTotal); err != nil {
			return nil, fmt.Errorf("expense repository scan category row: %w", err)
		}
		breakdown[category] = catTotal
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &domain.ExpenseReport{
		Period:    fmt.Sprintf("%d-%02d", year, month),
		Total:     total,
		Breakdown: breakdown,
		StartDate: start,
		EndDate:   end,
	}, nil
}

func (r *expenseRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM expenses WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("expense repository delete: %w", err)
	}
	return nil
}
