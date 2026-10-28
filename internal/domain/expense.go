package domain

import (
	"context"
	"time"
)

// Expense categories
const (
	CategoryFood          = "makan"
	CategoryTransport     = "transport"
	CategoryEntertainment = "hiburan"
	CategoryHealth        = "kesehatan"
	CategoryShopping      = "belanja"
	CategoryBill          = "tagihan"
	CategoryCharity       = "amal"
	CategoryOther         = "lainnya"
)

// Expense represents a spending record.
type Expense struct {
	ID          int64     `json:"id"`
	Amount      float64   `json:"amount"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	OccurredAt  time.Time `json:"occurred_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// ExpenseReport holds aggregated expense data.
type ExpenseReport struct {
	Period    string             `json:"period"`
	Total     float64            `json:"total"`
	Breakdown map[string]float64 `json:"breakdown"`
	StartDate time.Time          `json:"start_date"`
	EndDate   time.Time          `json:"end_date"`
}

// ExpenseRepository defines persistence operations for expenses.
type ExpenseRepository interface {
	Create(ctx context.Context, expense *Expense) error
	GetByID(ctx context.Context, id int64) (*Expense, error)
	GetByDateRange(ctx context.Context, start, end time.Time) ([]*Expense, error)
	GetMonthlyReport(ctx context.Context, year int, month int) (*ExpenseReport, error)
	Delete(ctx context.Context, id int64) error
}

// ExpenseUsecase defines business logic for expenses.
type ExpenseUsecase interface {
	RecordExpense(ctx context.Context, amount float64, category, description string, occurredAt time.Time) (*Expense, error)
	GetMonthlyReport(ctx context.Context, year, month int) (*ExpenseReport, error)
	GetDetailMonthlyReport(ctx context.Context, year, month int) ([]*Expense, error)
	GetDetailMonthReportByCategory(ctx context.Context, year, month int, category string) ([]*Expense, error)
	GetCurrentMonthReport(ctx context.Context) (*ExpenseReport, error)
	SendMonthlyReportIfDue(ctx context.Context) error
	DeleteExpense(ctx context.Context, id int64) error
	GetTodayExpenseReport(ctx context.Context) (*ExpenseReport, error)
	GetTodayExpenses(ctx context.Context) ([]*Expense, error)
	GetTodayExpensesByCategory(ctx context.Context, category string) ([]*Expense, error)
}
