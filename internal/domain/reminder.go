package domain

import (
	"context"
	"time"
)

// Reminder status constants
const (
	ReminderStatusActive    = "active"
	ReminderStatusDone      = "done"
	ReminderStatusCancelled = "cancelled"
)

// Recurrence patterns
const (
	RecurrenceDaily   = "daily"
	RecurrenceWeekly  = "weekly"
	RecurrenceMonthly = "monthly"
)

// Reminder represents a scheduled reminder entity.
type Reminder struct {
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	Recurrence  *string    `json:"recurrence,omitempty"` // nullable: daily/weekly/monthly/cron expr
	Status      string     `json:"status"`               // active/done/cancelled
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

// ReminderRepository defines persistence operations for reminders.
type ReminderRepository interface {
	Create(ctx context.Context, reminder *Reminder) error
	GetByID(ctx context.Context, id int64) (*Reminder, error)
	GetAllActive(ctx context.Context) ([]*Reminder, error)
	UpdateStatus(ctx context.Context, id int64, status string) error
	Update(ctx context.Context, reminder *Reminder) error
	Delete(ctx context.Context, id int64) error
	GetDueReminders(ctx context.Context, now time.Time) ([]*Reminder, error)
}

// ReminderUsecase defines business logic for reminders.
type ReminderUsecase interface {
	CreateReminder(ctx context.Context, title string, scheduledAt time.Time, recurrence *string) (*Reminder, error)
	ListActive(ctx context.Context) ([]*Reminder, error)
	Cancel(ctx context.Context, id int64) error
	Update(ctx context.Context, id int64, title string, scheduledAt time.Time) (*Reminder, error)
	ProcessDueReminders(ctx context.Context) error
}
