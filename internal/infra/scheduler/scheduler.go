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
// - Daily at 07:00 WIB: send proactive morning daily briefing
// - Daily at 20:00 WIB: check if monthly expense report should be sent
// - Daily at 20:05 WIB: check if monthly workout report should be sent
// - Daily at 21:00 WIB: send proactive nightly debrief
type Scheduler struct {
	cron       *cron.Cron
	reminderUC domain.ReminderUsecase
	expenseUC  domain.ExpenseUsecase
	workoutUC  domain.WorkoutUsecase
	briefingUC domain.BriefingUsecase
	logger     *zap.Logger
}

// NewScheduler creates and configures the cron scheduler with the given timezone.
func NewScheduler(
	reminderUC domain.ReminderUsecase,
	expenseUC domain.ExpenseUsecase,
	workoutUC domain.WorkoutUsecase,
	briefingUC domain.BriefingUsecase,
	logger *zap.Logger,
	tz *time.Location,
) *Scheduler {
	c := cron.New(cron.WithLocation(tz))
	s := &Scheduler{
		cron:       c,
		reminderUC: reminderUC,
		expenseUC:  expenseUC,
		workoutUC:  workoutUC,
		briefingUC: briefingUC,
		logger:     logger,
	}

	// Every minute: process due reminders
	_, err := c.AddFunc("* * * * *", s.processDueReminders)
	if err != nil {
		logger.Error("failed to add reminder cron job", zap.Error(err))
	}

	// Daily at 07:00 in tz: send morning daily briefing
	_, err = c.AddFunc("0 7 * * *", s.sendDailyBriefing)
	if err != nil {
		logger.Error("failed to add daily briefing cron job", zap.Error(err))
	}

	// Every day at 20:00 in tz: check if monthly expense report should be sent
	_, err = c.AddFunc("0 20 * * *", s.sendMonthlyExpenseReport)
	if err != nil {
		logger.Error("failed to add expense report cron job", zap.Error(err))
	}

	// Every day at 20:05 in tz: check if monthly workout report should be sent
	_, err = c.AddFunc("5 20 * * *", s.sendMonthlyWorkoutReport)
	if err != nil {
		logger.Error("failed to add workout report cron job", zap.Error(err))
	}

	// Daily at 21:00 in tz: send nightly debrief
	_, err = c.AddFunc("0 21 * * *", s.sendNightlyDebrief)
	if err != nil {
		logger.Error("failed to add nightly debrief cron job", zap.Error(err))
	}

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

func (s *Scheduler) sendDailyBriefing() {
	ctx := context.Background()
	s.logger.Info("triggering proactive daily briefing")
	if s.briefingUC != nil {
		if err := s.briefingUC.SendDailyBriefing(ctx); err != nil {
			s.logger.Error("send daily briefing error", zap.Error(err))
		}
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

func (s *Scheduler) sendNightlyDebrief() {
	ctx := context.Background()
	s.logger.Info("triggering proactive nightly debrief")
	if s.briefingUC != nil {
		if err := s.briefingUC.SendNightlyDebrief(ctx); err != nil {
			s.logger.Error("send nightly debrief error", zap.Error(err))
		}
	}
}
