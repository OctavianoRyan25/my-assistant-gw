package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
)

type reminderRepository struct {
	db *sql.DB
}

// NewReminderRepository creates a new MySQL-backed ReminderRepository.
func NewReminderRepository(db *sql.DB) domain.ReminderRepository {
	return &reminderRepository{db: db}
}

func (r *reminderRepository) Create(ctx context.Context, reminder *domain.Reminder) error {
	query := `
		INSERT INTO reminders (title, scheduled_at, recurrence, status, created_at)
		VALUES (?, ?, ?, ?, ?)
	`
	result, err := r.db.ExecContext(ctx, query,
		reminder.Title,
		reminder.ScheduledAt,
		reminder.Recurrence,
		reminder.Status,
		reminder.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("reminder repository create: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("reminder repository get last insert id: %w", err)
	}
	reminder.ID = id
	return nil
}

func (r *reminderRepository) GetByID(ctx context.Context, id int64) (*domain.Reminder, error) {
	query := `SELECT id, title, scheduled_at, recurrence, status, created_at, updated_at FROM reminders WHERE id = ?`
	row := r.db.QueryRowContext(ctx, query, id)
	return scanReminder(row)
}

func (r *reminderRepository) GetAllActive(ctx context.Context) ([]*domain.Reminder, error) {
	query := `SELECT id, title, scheduled_at, recurrence, status, created_at, updated_at FROM reminders WHERE status = 'active' ORDER BY scheduled_at ASC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("reminder repository get all active: %w", err)
	}
	defer rows.Close()
	return scanReminders(rows)
}

func (r *reminderRepository) GetDueReminders(ctx context.Context, now time.Time) ([]*domain.Reminder, error) {
	query := `
		SELECT id, title, scheduled_at, recurrence, status, created_at, updated_at
		FROM reminders
		WHERE status = 'active' AND scheduled_at <= ?
		ORDER BY scheduled_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, now)
	if err != nil {
		return nil, fmt.Errorf("reminder repository get due: %w", err)
	}
	defer rows.Close()
	return scanReminders(rows)
}

func (r *reminderRepository) UpdateStatus(ctx context.Context, id int64, status string) error {
	query := `UPDATE reminders SET status = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("reminder repository update status: %w", err)
	}
	return nil
}

func (r *reminderRepository) Update(ctx context.Context, reminder *domain.Reminder) error {
	query := `UPDATE reminders SET title = ?, scheduled_at = ?, recurrence = ?, status = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, reminder.Title, reminder.ScheduledAt, reminder.Recurrence, reminder.Status, reminder.ID)
	if err != nil {
		return fmt.Errorf("reminder repository update: %w", err)
	}
	return nil
}

func (r *reminderRepository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM reminders WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("reminder repository delete: %w", err)
	}
	return nil
}

// scanReminder scans a single row into a Reminder.
func scanReminder(row *sql.Row) (*domain.Reminder, error) {
	var rem domain.Reminder
	var recurrence sql.NullString
	var updatedAt sql.NullTime
	err := row.Scan(
		&rem.ID,
		&rem.Title,
		&rem.ScheduledAt,
		&recurrence,
		&rem.Status,
		&rem.CreatedAt,
		&updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan reminder: %w", err)
	}
	if recurrence.Valid {
		rem.Recurrence = &recurrence.String
	}
	if updatedAt.Valid {
		rem.UpdatedAt = &updatedAt.Time
	}
	return &rem, nil
}

// scanReminders scans multiple rows into a slice of Reminders.
func scanReminders(rows *sql.Rows) ([]*domain.Reminder, error) {
	var reminders []*domain.Reminder
	for rows.Next() {
		var rem domain.Reminder
		var recurrence sql.NullString
		var updatedAt sql.NullTime
		err := rows.Scan(
			&rem.ID,
			&rem.Title,
			&rem.ScheduledAt,
			&recurrence,
			&rem.Status,
			&rem.CreatedAt,
			&updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan reminders row: %w", err)
		}
		if recurrence.Valid {
			rem.Recurrence = &recurrence.String
		}
		if updatedAt.Valid {
			rem.UpdatedAt = &updatedAt.Time
		}
		reminders = append(reminders, &rem)
	}
	return reminders, rows.Err()
}
