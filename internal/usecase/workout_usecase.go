package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
	"go.uber.org/zap"
)

type workoutUsecase struct {
	repo     domain.WorkoutRepository
	parser   domain.WorkoutParser
	waSender domain.WhatsAppSender
	waPhone  string
	logger   *zap.Logger
	timezone *time.Location
}

// NewWorkoutUsecase creates a new WorkoutUsecase.
func NewWorkoutUsecase(
	repo domain.WorkoutRepository,
	parser domain.WorkoutParser,
	waSender domain.WhatsAppSender,
	waPhone string,
	logger *zap.Logger,
	timezone *time.Location,
) domain.WorkoutUsecase {
	return &workoutUsecase{
		repo:     repo,
		parser:   parser,
		waSender: waSender,
		waPhone:  waPhone,
		logger:   logger,
		timezone: timezone,
	}
}

func (u *workoutUsecase) RecordWorkout(ctx context.Context, workout *domain.Workout) (*domain.Workout, error) {
	if workout.Type == "" {
		workout.Type = domain.WorkoutTypeOther
	}
	if workout.LoggedAt.IsZero() {
		workout.LoggedAt = time.Now().In(u.timezone)
	}
	workout.CreatedAt = time.Now()

	if err := u.repo.Create(ctx, workout); err != nil {
		return nil, fmt.Errorf("workout usecase record: %w", err)
	}
	u.logger.Info("workout recorded",
		zap.Int64("id", workout.ID),
		zap.String("title", workout.Title),
		zap.String("type", workout.Type),
		zap.Int("calories", workout.CaloriesBurned),
	)
	return workout, nil
}

func (u *workoutUsecase) RecordWorkoutFromText(ctx context.Context, message string) (*domain.Workout, error) {
	if u.parser == nil {
		return nil, fmt.Errorf("workout parser not configured")
	}

	parsed, err := u.parser.ParseWorkoutFromText(ctx, message)
	if err != nil {
		return nil, fmt.Errorf("parse workout from text: %w", err)
	}
	if parsed == nil {
		return nil, fmt.Errorf("failed to extract workout information from text")
	}

	w := &domain.Workout{
		Title:           parsed.Title,
		Type:            parsed.Type,
		DurationMinutes: parsed.DurationMinutes,
		CaloriesBurned:  parsed.CaloriesBurned,
		DistanceMeters:  parsed.DistanceMeters,
		Description:     parsed.Description,
		LoggedAt:        time.Now().In(u.timezone),
	}
	return u.RecordWorkout(ctx, w)
}

func (u *workoutUsecase) RecordWorkoutFromImage(ctx context.Context, imageData []byte, mimeType, caption string) (*domain.Workout, error) {
	if u.parser == nil {
		return nil, fmt.Errorf("workout parser not configured")
	}

	parsed, err := u.parser.ParseWorkoutFromImage(ctx, imageData, mimeType, caption)
	if err != nil {
		return nil, fmt.Errorf("parse workout from image: %w", err)
	}
	if parsed == nil {
		return nil, fmt.Errorf("failed to extract workout information from image")
	}

	w := &domain.Workout{
		Title:           parsed.Title,
		Type:            parsed.Type,
		DurationMinutes: parsed.DurationMinutes,
		CaloriesBurned:  parsed.CaloriesBurned,
		DistanceMeters:  parsed.DistanceMeters,
		Description:     parsed.Description,
		LoggedAt:        time.Now().In(u.timezone),
	}
	return u.RecordWorkout(ctx, w)
}

func (u *workoutUsecase) GetMonthlyStats(ctx context.Context, year, month int) (*domain.WorkoutMonthlyStats, error) {
	return u.repo.GetMonthlyStats(ctx, year, month)
}

func (u *workoutUsecase) GetCurrentMonthStats(ctx context.Context) (*domain.WorkoutMonthlyStats, error) {
	now := time.Now().In(u.timezone)
	return u.repo.GetMonthlyStats(ctx, now.Year(), int(now.Month()))
}

func (u *workoutUsecase) GetRecentWorkouts(ctx context.Context, limit int) ([]*domain.Workout, error) {
	return u.repo.GetRecent(ctx, limit)
}

func (u *workoutUsecase) DeleteWorkout(ctx context.Context, id int64) (*domain.Workout, error) {
	workout, err := u.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("delete workout: get by id: %w", err)
	}
	if workout == nil {
		return nil, fmt.Errorf("workout not found")
	}

	if err := u.repo.Delete(ctx, id); err != nil {
		return nil, fmt.Errorf("delete workout: %w", err)
	}

	u.logger.Info("workout deleted",
		zap.Int64("id", workout.ID),
		zap.String("title", workout.Title),
	)
	return workout, nil
}

func (u *workoutUsecase) DeleteLatestWorkout(ctx context.Context) (*domain.Workout, error) {
	recent, err := u.repo.GetRecent(ctx, 1)
	if err != nil {
		return nil, fmt.Errorf("delete latest workout: fetch recent: %w", err)
	}
	if len(recent) == 0 {
		return nil, fmt.Errorf("no workouts found")
	}

	latest := recent[0]
	if err := u.repo.Delete(ctx, latest.ID); err != nil {
		return nil, fmt.Errorf("delete latest workout: %w", err)
	}

	u.logger.Info("latest workout deleted",
		zap.Int64("id", latest.ID),
		zap.String("title", latest.Title),
	)
	return latest, nil
}

func (u *workoutUsecase) SendMonthlyReportIfDue(ctx context.Context) error {
	now := time.Now().In(u.timezone)
	// Check if today is the last day of the month
	tomorrow := now.AddDate(0, 0, 1)
	if tomorrow.Day() != 1 {
		return nil
	}

	stats, err := u.repo.GetMonthlyStats(ctx, now.Year(), int(now.Month()))
	if err != nil {
		return fmt.Errorf("send monthly workout report: fetch stats: %w", err)
	}

	msg := FormatWorkoutReport(stats)
	if err := u.waSender.SendMessage(ctx, u.waPhone, msg); err != nil {
		return fmt.Errorf("send monthly workout report: wa send: %w", err)
	}
	u.logger.Info("monthly workout report sent", zap.String("period", stats.Period))
	return nil
}

// FormatWorkoutList returns a nicely formatted WhatsApp message for recent workouts.
func FormatWorkoutList(workouts []*domain.Workout) string {
	if len(workouts) == 0 {
		return "Belum ada riwayat workout yang tercatat wir 🏋️‍♂️. Yuk mulai olahraga!"
	}

	var sb strings.Builder
	sb.WriteString("🏋️ *Riwayat Catatan Workout:*\n\n")

	dayNames := map[string]string{
		"Sunday":    "Min",
		"Monday":    "Sen",
		"Tuesday":   "Sel",
		"Wednesday": "Rab",
		"Thursday":  "Kam",
		"Friday":    "Jum",
		"Saturday":  "Sab",
	}
	monthNames := map[string]string{
		"January":   "Jan",
		"February":  "Feb",
		"March":     "Mar",
		"April":     "Apr",
		"May":       "Mei",
		"June":      "Jun",
		"July":      "Jul",
		"August":    "Agt",
		"September": "Sep",
		"October":   "Okt",
		"November":  "Nov",
		"December":  "Des",
	}

	for i, w := range workouts {
		day := dayNames[w.LoggedAt.Format("Monday")]
		if day == "" {
			day = w.LoggedAt.Format("Mon")
		}
		month := monthNames[w.LoggedAt.Format("January")]
		if month == "" {
			month = w.LoggedAt.Format("Jan")
		}
		dateStr := fmt.Sprintf("%s, %02d %s %s %s", day, w.LoggedAt.Day(), month, w.LoggedAt.Format("2006"), w.LoggedAt.Format("15:04"))

		sb.WriteString(fmt.Sprintf("%d. *%s* (ID: %d)\n", i+1, w.Title, w.ID))
		sb.WriteString(fmt.Sprintf("   📅 %s\n", dateStr))

		var details []string
		if w.Type != "" {
			details = append(details, fmt.Sprintf("🏷️ %s", w.Type))
		}
		if w.DurationMinutes > 0 {
			details = append(details, fmt.Sprintf("⏱️ %d mnt", w.DurationMinutes))
		}
		if w.CaloriesBurned > 0 {
			details = append(details, fmt.Sprintf("🔥 ~%d kcal", w.CaloriesBurned))
		}
		if w.DistanceMeters > 0 {
			if w.DistanceMeters >= 1000 {
				details = append(details, fmt.Sprintf("🏃 %.2f km", w.DistanceMeters/1000.0))
			} else {
				details = append(details, fmt.Sprintf("🏃 %.0f m", w.DistanceMeters))
			}
		}
		if len(details) > 0 {
			sb.WriteString(fmt.Sprintf("   %s\n", strings.Join(details, " • ")))
		}
		if w.Description != "" {
			sb.WriteString(fmt.Sprintf("   📝 %s\n", w.Description))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("_Ketik *hapus workout <ID>* atau *hapus workout terakhir* untuk menghapus catatan._")
	return strings.TrimSpace(sb.String())
}

// FormatWorkoutConfirmation returns a friendly confirmation message.
func FormatWorkoutConfirmation(workout *domain.Workout) string {
	var sb strings.Builder
	sb.WriteString("🔥 *Workout berhasil dicatat wir! Mantap!*\n\n")
	sb.WriteString(fmt.Sprintf("• Aktivitas: *%s*\n", workout.Title))
	sb.WriteString(fmt.Sprintf("• Kategori: %s\n", workout.Type))
	if workout.DurationMinutes > 0 {
		sb.WriteString(fmt.Sprintf("• Durasi: %d menit\n", workout.DurationMinutes))
	}
	if workout.CaloriesBurned > 0 {
		sb.WriteString(fmt.Sprintf("• Kalori terbakar: ~%d kcal 🔥\n", workout.CaloriesBurned))
	}
	if workout.DistanceMeters > 0 {
		if workout.DistanceMeters >= 1000 {
			sb.WriteString(fmt.Sprintf("• Jarak: %.2f km\n", workout.DistanceMeters/1000.0))
		} else {
			sb.WriteString(fmt.Sprintf("• Jarak: %.0f m\n", workout.DistanceMeters))
		}
	}
	if workout.Description != "" {
		sb.WriteString(fmt.Sprintf("• Catatan: %s\n", workout.Description))
	}
	sb.WriteString("\nKonsistensi adalah kunci! Pertahankan semangatnya 💪🏃")
	return sb.String()
}

// FormatWorkoutReport returns a readable monthly recap for WhatsApp.
func FormatWorkoutReport(stats *domain.WorkoutMonthlyStats) string {
	if stats == nil || stats.TotalSessions == 0 {
		return "Belum ada catatan workout untuk periode ini. Yuk mulai gerak wir! 🏋️"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 *Statistik Workout Bulanan (%s)*\n\n", stats.Period))
	sb.WriteString(fmt.Sprintf("• Total Sesi: *%d kali*\n", stats.TotalSessions))
	sb.WriteString(fmt.Sprintf("• Total Durasi: *%d menit* (%.1f jam)\n", stats.TotalDurationMinutes, float64(stats.TotalDurationMinutes)/60.0))
	if stats.TotalCaloriesBurned > 0 {
		sb.WriteString(fmt.Sprintf("• Total Kalori Terbakar: *%d kcal* 🔥\n", stats.TotalCaloriesBurned))
	}

	if len(stats.TypeBreakdown) > 0 {
		sb.WriteString("\n🏅 *Aktivitas Olahraga:*\n")
		for t, count := range stats.TypeBreakdown {
			sb.WriteString(fmt.Sprintf("• %s: %d sesi\n", strings.Title(t), count))
		}
	}

	sb.WriteString("\nKerja keras bulan ini luar biasa, teruskan gaya hidup sehatnya! 🚀")
	return sb.String()
}
