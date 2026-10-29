package usecase

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
	"go.uber.org/zap"
)

type briefingUsecase struct {
	reminderRepo  domain.ReminderRepository
	expenseUC     domain.ExpenseUsecase
	budgetUC      domain.BudgetUsecase
	nutritionUC   domain.NutritionUsecase
	workoutRepo   domain.WorkoutRepository
	userProfileUC domain.UserProfileUsecase
	waSender      domain.WhatsAppSender
	waPhone       string
	logger        *zap.Logger
	timezone      *time.Location
}

// NewBriefingUsecase creates a new BriefingUsecase instance.
func NewBriefingUsecase(
	reminderRepo domain.ReminderRepository,
	expenseUC domain.ExpenseUsecase,
	budgetUC domain.BudgetUsecase,
	nutritionUC domain.NutritionUsecase,
	workoutRepo domain.WorkoutRepository,
	userProfileUC domain.UserProfileUsecase,
	waSender domain.WhatsAppSender,
	waPhone string,
	logger *zap.Logger,
	timezone *time.Location,
) domain.BriefingUsecase {
	return &briefingUsecase{
		reminderRepo:  reminderRepo,
		expenseUC:     expenseUC,
		budgetUC:      budgetUC,
		nutritionUC:   nutritionUC,
		workoutRepo:   workoutRepo,
		userProfileUC: userProfileUC,
		waSender:      waSender,
		waPhone:       waPhone,
		logger:        logger,
		timezone:      timezone,
	}
}

// GetDailyBriefing gathers data and formats the morning proactive briefing message.
func (u *briefingUsecase) GetDailyBriefing(ctx context.Context) (string, error) {
	now := time.Now().In(u.timezone)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, u.timezone)
	endOfDay := startOfDay.AddDate(0, 0, 1).Add(-time.Nanosecond)

	briefing := &domain.DailyBriefing{
		Date: now,
	}

	// 1. Reminders
	if u.reminderRepo != nil {
		allActive, err := u.reminderRepo.GetAllActive(ctx)
		if err != nil {
			u.logger.Warn("failed to fetch active reminders for briefing", zap.Error(err))
		} else {
			for _, r := range allActive {
				remTime := r.ScheduledAt.In(u.timezone)
				if remTime.Before(startOfDay) {
					briefing.OverdueReminders = append(briefing.OverdueReminders, r)
				} else if !remTime.After(endOfDay) {
					briefing.TodayReminders = append(briefing.TodayReminders, r)
				}
			}
		}
	}

	// 2. Budget & Safe Daily Spend
	if u.budgetUC != nil {
		status, err := u.budgetUC.GetBudgetStatus(ctx, now.Year(), int(now.Month()))
		if err != nil {
			u.logger.Warn("failed to fetch budget status for briefing", zap.Error(err))
		} else if status != nil {
			briefing.BudgetStatus = status

			firstOfNextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, u.timezone)
			lastDayOfMonth := firstOfNextMonth.Add(-24 * time.Hour).Day()
			daysRemaining := lastDayOfMonth - now.Day() + 1
			briefing.DaysRemaining = daysRemaining

			if status.Remaining > 0 && daysRemaining > 0 {
				briefing.SafeDailySpend = status.Remaining / float64(daysRemaining)
			}
		}
	}

	// 3. User Profile
	if u.userProfileUC != nil {
		profile, err := u.userProfileUC.GetProfile(ctx)
		if err != nil {
			u.logger.Warn("failed to fetch user profile for briefing", zap.Error(err))
		} else {
			briefing.Profile = profile
		}
	}

	// 4. Nutrition Target
	if u.nutritionUC != nil {
		nutritionSummary, err := u.nutritionUC.GetDailySummary(ctx, now)
		if err != nil {
			u.logger.Warn("failed to fetch nutrition summary for briefing", zap.Error(err))
		} else {
			briefing.NutritionSummary = nutritionSummary
		}
	}

	return formatDailyBriefing(briefing, u.timezone), nil
}

// SendDailyBriefing formats the morning briefing and dispatches it via WhatsApp.
func (u *briefingUsecase) SendDailyBriefing(ctx context.Context) error {
	if u.waSender == nil || u.waPhone == "" {
		u.logger.Warn("whatsapp sender or phone number not configured; skipping briefing dispatch")
		return nil
	}

	msg, err := u.GetDailyBriefing(ctx)
	if err != nil {
		return fmt.Errorf("generate daily briefing: %w", err)
	}

	if err := u.waSender.SendMessage(ctx, u.waPhone, msg); err != nil {
		return fmt.Errorf("send daily briefing message: %w", err)
	}

	u.logger.Info("daily briefing dispatched successfully", zap.String("to", u.waPhone))
	return nil
}

// GetNightlyDebrief gathers data and formats the evening recap message.
func (u *briefingUsecase) GetNightlyDebrief(ctx context.Context) (string, error) {
	now := time.Now().In(u.timezone)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, u.timezone)
	endOfDay := startOfDay.AddDate(0, 0, 1).Add(-time.Nanosecond)

	debrief := &domain.NightlyDebrief{
		Date: now,
	}

	// 1. Today's Expenses
	if u.expenseUC != nil {
		expenses, err := u.expenseUC.GetTodayExpenses(ctx)
		if err != nil {
			u.logger.Warn("failed to fetch today expenses for debrief", zap.Error(err))
		} else {
			debrief.TodayExpenses = expenses
		}

		report, err := u.expenseUC.GetTodayExpenseReport(ctx)
		if err != nil {
			u.logger.Warn("failed to fetch today expense report for debrief", zap.Error(err))
		} else {
			debrief.ExpenseReport = report
		}
	}

	// 2. Budget Status
	if u.budgetUC != nil {
		status, err := u.budgetUC.GetBudgetStatus(ctx, now.Year(), int(now.Month()))
		if err == nil {
			debrief.BudgetStatus = status
		}
	}

	// 3. Nutrition Summary
	if u.nutritionUC != nil {
		nutritionSummary, err := u.nutritionUC.GetDailySummary(ctx, now)
		if err == nil {
			debrief.NutritionSummary = nutritionSummary
		}
	}

	// 4. Workouts logged today
	if u.workoutRepo != nil {
		workouts, err := u.workoutRepo.GetByDateRange(ctx, startOfDay, endOfDay)
		if err != nil {
			u.logger.Warn("failed to fetch today workouts for debrief", zap.Error(err))
		} else {
			debrief.Workouts = workouts
			for _, w := range workouts {
				debrief.TotalDurationMin += w.DurationMinutes
				debrief.TotalBurnedKcal += w.CaloriesBurned
			}
		}
	}

	// 5. Active/pending reminders for today or overdue
	if u.reminderRepo != nil {
		activeReminders, err := u.reminderRepo.GetAllActive(ctx)
		if err == nil {
			for _, r := range activeReminders {
				remTime := r.ScheduledAt.In(u.timezone)
				if !remTime.After(endOfDay) {
					debrief.PendingReminders = append(debrief.PendingReminders, r)
				}
			}
		}
	}

	return formatNightlyDebrief(debrief, u.timezone), nil
}

// SendNightlyDebrief formats the nightly debrief and dispatches it via WhatsApp.
func (u *briefingUsecase) SendNightlyDebrief(ctx context.Context) error {
	if u.waSender == nil || u.waPhone == "" {
		u.logger.Warn("whatsapp sender or phone number not configured; skipping debrief dispatch")
		return nil
	}

	msg, err := u.GetNightlyDebrief(ctx)
	if err != nil {
		return fmt.Errorf("generate nightly debrief: %w", err)
	}

	if err := u.waSender.SendMessage(ctx, u.waPhone, msg); err != nil {
		return fmt.Errorf("send nightly debrief message: %w", err)
	}

	u.logger.Info("nightly debrief dispatched successfully", zap.String("to", u.waPhone))
	return nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func formatIndonesianDate(t time.Time) string {
	days := [...]string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}
	months := [...]string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	return fmt.Sprintf("%s, %d %s %d", days[t.Weekday()], t.Day(), months[t.Month()], t.Year())
}

func formatDailyBriefing(b *domain.DailyBriefing, tz *time.Location) string {
	var sb strings.Builder
	dateStr := formatIndonesianDate(b.Date)

	sb.WriteString(fmt.Sprintf("🌅 *DAILY BRIEFING — %s*\n", dateStr))
	sb.WriteString("Semangat pagi Jawa! Ini rencana dan panduan harimu:\n\n")

	// 1. Reminders
	sb.WriteString("📌 *Agenda & Pengingat Hari Ini:*\n")
	if len(b.TodayReminders) == 0 {
		sb.WriteString("• _Tidak ada agenda tersimpan untuk hari ini. Waktu fleksibel! ✨_\n")
	} else {
		for _, r := range b.TodayReminders {
			t := r.ScheduledAt.In(tz).Format("15:04")
			sb.WriteString(fmt.Sprintf("• [%s] %s\n", t, r.Title))
		}
	}

	if len(b.OverdueReminders) > 0 {
		sb.WriteString("\n⚠️ *Catatan Pengingat Terlewat:*\n")
		for _, r := range b.OverdueReminders {
			t := r.ScheduledAt.In(tz).Format("02 Jan 15:04")
			sb.WriteString(fmt.Sprintf("• (Jatuh tempo: %s) %s\n", t, r.Title))
		}
	}

	// 2. Budget & Financial Health
	sb.WriteString("\n💰 *Kondisi Keuangan Bulan Ini:*\n")
	if b.BudgetStatus != nil && b.BudgetStatus.MonthlySalary > 0 {
		if b.BudgetStatus.IsExceeded {
			sb.WriteString(fmt.Sprintf("• Status: 🛑 *Woi ireng, ngirit!* (-Rp %s)\n", formatRupiah(-b.BudgetStatus.Remaining)))
			sb.WriteString("• Saran: Rem pengeluaran non-primer hari ini ya wir!\n")
		} else {
			sb.WriteString(fmt.Sprintf("• Sisa Budget: *Rp %s* (Terpakai %.1f%%)\n", formatRupiah(b.BudgetStatus.Remaining), b.BudgetStatus.SpentPercentage))
			if b.SafeDailySpend > 0 {
				sb.WriteString(fmt.Sprintf("• Batas Belanja Aman: *~Rp %s / hari* (sisa %d hari)\n", formatRupiah(b.SafeDailySpend), b.DaysRemaining))
			}
		}
	} else {
		sb.WriteString("• Budget belum diatur. Ketik *set budget <jumlah>* untuk memantau pengeluaran bulanan.\n")
	}

	// 3. Nutrition & Physical Target
	sb.WriteString("\n🥗 *Kesehatan & Nutrisi:*\n")
	targetCal := 2000
	if b.Profile != nil && b.Profile.DailyCalorieTarget > 0 {
		targetCal = b.Profile.DailyCalorieTarget
		sb.WriteString(fmt.Sprintf("• Target Kalori Harian: *%d kcal* (TB: %.0f cm, BB: %.0f kg)\n", targetCal, b.Profile.HeightCM, b.Profile.WeightKG))
	} else {
		sb.WriteString(fmt.Sprintf("• Target Kalori Harian: *~%d kcal*\n", targetCal))
	}
	sb.WriteString("• Tips: Awali hari dengan sarapan berprotein dan cukupi air putih minimal 2L! 💧\n")

	// 4. Workout Advice
	sb.WriteString("\n🏋️ *Aktivitas Olahraga:*\n")
	sb.WriteString("• Rencanakan 30 menit sesi gerak hari ini. Setelah selesai, jangan lupa foto/ketik *catat workout <aktivitas>* ke asisten! 💪\n")

	sb.WriteString("\n_Semoga harimu produktif dan menyenangkan wir! Ada yang mau dibantu?_ 🚀")
	return sb.String()
}

func formatNightlyDebrief(d *domain.NightlyDebrief, tz *time.Location) string {
	var sb strings.Builder
	dateStr := formatIndonesianDate(d.Date)

	sb.WriteString(fmt.Sprintf("🌙 *NIGHTLY DEBRIEF — %s*\n", dateStr))
	sb.WriteString("Kerja Bagus! Ini rekap dan evaluasimu:\n\n")

	// 1. Today's Expenses
	sb.WriteString("💸 *Pengeluaran Hari Ini:*\n")
	if d.ExpenseReport != nil && d.ExpenseReport.Total > 0 {
		count := len(d.TodayExpenses)
		if count > 0 {
			sb.WriteString(fmt.Sprintf("• Total: *Rp %s* (%d transaksi)\n", formatRupiah(d.ExpenseReport.Total), count))
		} else {
			sb.WriteString(fmt.Sprintf("• Total: *Rp %s*\n", formatRupiah(d.ExpenseReport.Total)))
		}

		if len(d.ExpenseReport.Breakdown) > 0 {
			// Sort categories by amount desc
			type catItem struct {
				name   string
				amount float64
			}
			cats := make([]catItem, 0, len(d.ExpenseReport.Breakdown))
			for name, amount := range d.ExpenseReport.Breakdown {
				cats = append(cats, catItem{name, amount})
			}
			sort.Slice(cats, func(i, j int) bool {
				return cats[i].amount > cats[j].amount
			})
			for _, c := range cats {
				sb.WriteString(fmt.Sprintf("  - %s: Rp %s\n", c.name, formatRupiah(c.amount)))
			}
		}
		if d.BudgetStatus != nil && d.BudgetStatus.MonthlySalary > 0 {
			if d.BudgetStatus.IsExceeded {
				sb.WriteString(fmt.Sprintf("• Sisa Budget Bulan Ini: 🛑 *Melebihi budget Rp %s*\n", formatRupiah(-d.BudgetStatus.Remaining)))
			} else {
				sb.WriteString(fmt.Sprintf("• Sisa Budget Bulan Ini: *Rp %s* (%.1f%% terpakai)\n", formatRupiah(d.BudgetStatus.Remaining), d.BudgetStatus.SpentPercentage))
			}
		}
	} else {
		sb.WriteString("• *Rp 0* (Hari hemat tanpa pengeluaran! Dompet aman terkendali 👏)\n")
	}

	// 2. Nutrition Recap
	sb.WriteString("\n🥗 *Nutrisi & Asupan Kalori:*\n")
	if d.NutritionSummary != nil && d.NutritionSummary.TotalCalories > 0 {
		sb.WriteString(fmt.Sprintf("• Asupan: *%d / %d kcal* ", d.NutritionSummary.TotalCalories, d.NutritionSummary.TargetCalories))
		if d.NutritionSummary.RemainingCalories >= 0 {
			sb.WriteString("(On-Track ✅)\n")
		} else {
			sb.WriteString(fmt.Sprintf("(Melebihi target %d kcal ⚠️)\n", -d.NutritionSummary.RemainingCalories))
		}
		if d.NutritionSummary.TotalProtein > 0 || d.NutritionSummary.TotalCarbs > 0 || d.NutritionSummary.TotalFat > 0 {
			sb.WriteString(fmt.Sprintf("• Makro: P: %.0fg • K: %.0fg • L: %.0fg\n", d.NutritionSummary.TotalProtein, d.NutritionSummary.TotalCarbs, d.NutritionSummary.TotalFat))
		}
	} else {
		sb.WriteString("• _Belum ada makanan yang dicatat hari ini. Jika tadi makan berat, kamu masih bisa kirim foto atau ketik ke asisten._\n")
	}

	// 3. Workouts
	sb.WriteString("\n🏋️ *Aktivitas Olahraga:*\n")
	if len(d.Workouts) > 0 {
		sb.WriteString(fmt.Sprintf("• *%d Sesi Latihan Tercatat:* (Total: %d menit, ~%d kcal 🔥)\n", len(d.Workouts), d.TotalDurationMin, d.TotalBurnedKcal))
		for _, w := range d.Workouts {
			details := []string{}
			if w.DurationMinutes > 0 {
				details = append(details, fmt.Sprintf("%d menit", w.DurationMinutes))
			}
			if w.CaloriesBurned > 0 {
				details = append(details, fmt.Sprintf("~%d kcal", w.CaloriesBurned))
			}
			if w.DistanceMeters > 0 {
				if w.DistanceMeters >= 1000 {
					details = append(details, fmt.Sprintf("%.2f km", w.DistanceMeters/1000.0))
				} else {
					details = append(details, fmt.Sprintf("%.0f m", w.DistanceMeters))
				}
			}
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", w.Title, strings.Join(details, " • ")))
		}
	} else {
		sb.WriteString("• 🛋️ _Belum ada sesi olahraga hari ini. Istirahat yang cukup untuk recovery tubuhmu!_\n")
	}

	// 4. Pending Reminders
	if len(d.PendingReminders) > 0 {
		sb.WriteString("\n📌 *Catatan Pengingat yang Masih Aktif:*\n")
		for _, r := range d.PendingReminders {
			sb.WriteString(fmt.Sprintf("• %s\n", r.Title))
		}
	}

	sb.WriteString("\n_Selamat beristirahat dan tidur nyenyak wir! Sampai jumpa besok pagi 😴✨_")
	return sb.String()
}
