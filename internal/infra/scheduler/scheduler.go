package scheduler

import (
	"context"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// Scheduler wraps robfig/cron to run periodic tasks:
// - Every minute: check due reminders
// - Daily at 20:00 WIB: check if monthly expense report should be sent
type Scheduler struct {
	cron       *cron.Cron
	reminderUC domain.ReminderUsecase
	expenseUC  domain.ExpenseUsecase
	workoutUC  domain.WorkoutUsecase
	logger     *zap.Logger
}

// NewScheduler creates and configures the cron scheduler with the given timezone.
func NewScheduler(reminderUC domain.ReminderUsecase, expenseUC domain.ExpenseUsecase, workoutUC domain.WorkoutUsecase, logger *zap.Logger, tz *time.Location) *Scheduler {
	c := cron.New(cron.WithLocation(tz))
	s := &Scheduler{
		cron:       c,
		reminderUC: reminderUC,
		expenseUC:  expenseUC,
		workoutUC:  workoutUC,
		logger:     logger,
	}

	// Every minute: process due reminders
	_, err := c.AddFunc("* * * * *", s.processDueReminders)
	if err != nil {
		logger.Error("failed to add reminder cron job", zap.Error(err))
	}

	// Every day at 20:00 in tz: check if monthly expense report should be sent
	_, err = c.AddFunc("0 20 * * *", s.sendMonthlyExpenseReport)
	if err != nil {
		logger.Error("failed to add expense report cron job", zap.Error(err))
	}

	// Every day at 20:00 in tz: check if monthly workout report should be sent
	_, err = c.AddFunc("0 20 * * *", s.sendMonthlyWorkoutReport)
	if err != nil {
		logger.Error("failed to add workout report cron job", zap.Error(err))
	}

	// Every day of month 15 18:00 WIB: check if budget report
	_, err = c.AddFunc("0 18 15 * *", func() {
		logger.Info("schedular at 18:00 day of 15, month: check if budget report")
	})
	// TODO: add send monthly budget report cron job
	// if err != nil {
	// 	logger.Error("failed to add workout report cron job", zap.Error(err))
	// }

	return s
}

// Start begins the scheduler in the background.
func (s *Scheduler) Start() {
	s.cron.Start()
	s.logger.Info("scheduler started")
}

// Stop gracefully stops the scheduler.
func (s *Scheduler) Stop() {
	s.cron.Stop()
	s.logger.Info("scheduler stopped")
}

func (s *Scheduler) processDueReminders() {
	// Use a background context for scheduled jobs — not tied to any HTTP request
	ctx := context.Background()
	s.logger.Debug("checking due reminders")
	if err := s.reminderUC.ProcessDueReminders(ctx); err != nil {
		s.logger.Error("process due reminders error", zap.Error(err))
	}
}

func (s *Scheduler) sendMonthlyExpenseReport() {
	ctx := context.Background()
	s.logger.Info("checking monthly expense report")
	if err := s.expenseUC.SendMonthlyReportIfDue(ctx); err != nil {
		s.logger.Error("send monthly expense report error", zap.Error(err))
	}
}

// SendMonthlyWorkoutReport
func (s *Scheduler) sendMonthlyWorkoutReport() {
	ctx := context.Background()
	s.logger.Info("checking monthly workout report")
	if err := s.workoutUC.SendMonthlyReportIfDue(ctx); err != nil {
		s.logger.Error("send monthly workout report error", zap.Error(err))
	}
}
