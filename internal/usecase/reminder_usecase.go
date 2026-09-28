package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
	"go.uber.org/zap"
)

type reminderUsecase struct {
	repo     domain.ReminderRepository
	llm      domain.LLMClient
	waSender domain.WhatsAppSender
	waPhone  string
	logger   *zap.Logger
	timezone *time.Location
}

// NewReminderUsecase creates a new ReminderUsecase.
func NewReminderUsecase(
	repo domain.ReminderRepository,
	llm domain.LLMClient,
	waSender domain.WhatsAppSender,
	waPhone string,
	logger *zap.Logger,
	timezone *time.Location,
) domain.ReminderUsecase {
	return &reminderUsecase{
		repo:     repo,
		llm:      llm,
		waSender: waSender,
		waPhone:  waPhone,
		logger:   logger,
		timezone: timezone,
	}
}

// CreateReminder creates a new reminder directly with parsed data.
func (u *reminderUsecase) CreateReminder(ctx context.Context, title string, scheduledAt time.Time, recurrence *string) (*domain.Reminder, error) {
	reminder := &domain.Reminder{
		Title:       title,
		ScheduledAt: scheduledAt,
		Recurrence:  recurrence,
		Status:      domain.ReminderStatusActive,
		CreatedAt:   time.Now(),
	}
	if err := u.repo.Create(ctx, reminder); err != nil {
		return nil, fmt.Errorf("create reminder: %w", err)
	}
	u.logger.Info("reminder created", zap.Int64("id", reminder.ID), zap.String("title", title), zap.Time("scheduled_at", scheduledAt))
	return reminder, nil
}

// ListActive returns all active reminders.
func (u *reminderUsecase) ListActive(ctx context.Context) ([]*domain.Reminder, error) {
	return u.repo.GetAllActive(ctx)
}

// Cancel marks a reminder as cancelled.
func (u *reminderUsecase) Cancel(ctx context.Context, id int64) error {
	if err := u.repo.UpdateStatus(ctx, id, domain.ReminderStatusCancelled); err != nil {
		return fmt.Errorf("cancel reminder %d: %w", id, err)
	}
	u.logger.Info("reminder cancelled", zap.Int64("id", id))
	return nil
}

// Update modifies a reminder's title and schedule.
func (u *reminderUsecase) Update(ctx context.Context, id int64, title string, scheduledAt time.Time) (*domain.Reminder, error) {
	existing, err := u.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("update reminder: fetch existing: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("reminder %d not found", id)
	}

	existing.Title = title
	existing.ScheduledAt = scheduledAt

	if err := u.repo.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("update reminder %d: %w", id, err)
	}
	return existing, nil
}

// ProcessDueReminders fetches all due reminders, sends WA notifications,
// and reschedules recurring ones or marks one-time ones as done.
func (u *reminderUsecase) ProcessDueReminders(ctx context.Context) error {
	now := time.Now().In(u.timezone)
	due, err := u.repo.GetDueReminders(ctx, now)
	if err != nil {
		return fmt.Errorf("process due reminders fetch: %w", err)
	}

	for _, rem := range due {
		// Send WhatsApp notification
		msg := fmt.Sprintf("⏰ *Reminder:* %s", rem.Title)
		if err := u.waSender.SendMessage(ctx, u.waPhone, msg); err != nil {
			u.logger.Error("failed to send reminder notification", zap.Int64("id", rem.ID), zap.Error(err))
			continue
		}

		// Handle recurrence or mark as done
		if rem.Recurrence != nil && *rem.Recurrence != "" {
			nextAt, err := nextOccurrence(*rem.Recurrence, rem.ScheduledAt)
			if err != nil {
				u.logger.Error("failed to compute next occurrence", zap.Int64("id", rem.ID), zap.Error(err))
				// Mark as done if we can't compute next
				_ = u.repo.UpdateStatus(ctx, rem.ID, domain.ReminderStatusDone)
				continue
			}
			rem.ScheduledAt = nextAt
			if err := u.repo.Update(ctx, rem); err != nil {
				u.logger.Error("failed to reschedule reminder", zap.Int64("id", rem.ID), zap.Error(err))
			} else {
				u.logger.Info("reminder rescheduled", zap.Int64("id", rem.ID), zap.Time("next_at", nextAt))
			}
		} else {
			// One-time reminder — mark as done
			if err := u.repo.UpdateStatus(ctx, rem.ID, domain.ReminderStatusDone); err != nil {
				u.logger.Error("failed to mark reminder done", zap.Int64("id", rem.ID), zap.Error(err))
			}
		}
	}
	return nil
}

// nextOccurrence computes the next scheduled time based on recurrence pattern.
func nextOccurrence(recurrence string, from time.Time) (time.Time, error) {
	switch recurrence {
	case domain.RecurrenceDaily:
		return from.AddDate(0, 0, 1), nil
	case domain.RecurrenceWeekly:
		return from.AddDate(0, 0, 7), nil
	case domain.RecurrenceMonthly:
		return from.AddDate(0, 1, 0), nil
	default:
		return time.Time{}, fmt.Errorf("unknown recurrence pattern: %s", recurrence)
	}
}

// FormatReminderList formats a list of reminders into a readable WA message.
func FormatReminderList(reminders []*domain.Reminder) string {
	if len(reminders) == 0 {
		return "Tidak ada reminder aktif saat ini. 👌"
	}
	msg := "📋 *Reminder aktifmu:*\n\n"
	for i, r := range reminders {
		recStr := "sekali"
		if r.Recurrence != nil {
			recStr = *r.Recurrence
		}
		msg += fmt.Sprintf("%d. %s\n   ⏰ %s (%s)\n\n",
			i+1,
			r.Title,
			r.ScheduledAt.Format("Mon, 02 Jan 2006 15:04 WIB"),
			recStr,
		)
	}
	return msg
}
