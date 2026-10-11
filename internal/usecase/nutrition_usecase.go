package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
	"go.uber.org/zap"
)

type nutritionUsecase struct {
	repo       domain.NutritionRepository
	profileUC  domain.UserProfileUsecase
	parser     domain.NutritionParser
	logger     *zap.Logger
	timezone   *time.Location
}

// NewNutritionUsecase creates a new NutritionUsecase.
func NewNutritionUsecase(
	repo domain.NutritionRepository,
	profileUC domain.UserProfileUsecase,
	parser domain.NutritionParser,
	logger *zap.Logger,
	timezone *time.Location,
) domain.NutritionUsecase {
	return &nutritionUsecase{
		repo:      repo,
		profileUC: profileUC,
		parser:    parser,
		logger:    logger,
		timezone:  timezone,
	}
}

func (u *nutritionUsecase) LogMealFromImage(ctx context.Context, imageData []byte, mimeType string, notes string) (*domain.FoodLog, *domain.DailyNutritionSummary, error) {
	if u.parser == nil {
		return nil, nil, fmt.Errorf("nutrition vision parser not configured")
	}

	parsed, err := u.parser.ParseMealFromImage(ctx, imageData, mimeType)
	if err != nil {
		return nil, nil, fmt.Errorf("parse meal from image: %w", err)
	}
	if parsed == nil {
		return nil, nil, fmt.Errorf("could not detect food from image")
	}

	mealNotes := notes
	if mealNotes == "" {
		mealNotes = parsed.PortionDescription
	} else if parsed.PortionDescription != "" {
		mealNotes = fmt.Sprintf("%s (%s)", mealNotes, parsed.PortionDescription)
	}

	now := time.Now().In(u.timezone)
	foodLog := &domain.FoodLog{
		FoodName:          parsed.FoodName,
		EstimatedCalories: parsed.EstimatedCalories,
		Protein:           parsed.Protein,
		Carbs:             parsed.Carbs,
		Fat:               parsed.Fat,
		Confidence:        parsed.Confidence,
		Notes:             mealNotes,
		LoggedAt:          now,
		CreatedAt:         time.Now(),
	}

	if err := u.repo.Create(ctx, foodLog); err != nil {
		return nil, nil, fmt.Errorf("save food log: %w", err)
	}
	u.logger.Info("meal logged from image",
		zap.Int64("id", foodLog.ID),
		zap.String("food", foodLog.FoodName),
		zap.Int("calories", foodLog.EstimatedCalories),
	)

	summary, err := u.GetDailySummary(ctx, now)
	if err != nil {
		u.logger.Warn("failed to fetch daily summary after logging meal", zap.Error(err))
	} else if parsed.Advice != "" && summary != nil {
		summary.Advice = parsed.Advice
	}

	return foodLog, summary, nil
}

func (u *nutritionUsecase) LogMealFromText(ctx context.Context, foodName string, calories int, notes string, loggedAt time.Time) (*domain.FoodLog, *domain.DailyNutritionSummary, error) {
	if loggedAt.IsZero() {
		loggedAt = time.Now().In(u.timezone)
	}

	foodLog := &domain.FoodLog{
		FoodName:          foodName,
		EstimatedCalories: calories,
		Notes:             notes,
		LoggedAt:          loggedAt,
		CreatedAt:         time.Now(),
	}

	if err := u.repo.Create(ctx, foodLog); err != nil {
		return nil, nil, fmt.Errorf("save food log from text: %w", err)
	}
	u.logger.Info("meal logged from text",
		zap.Int64("id", foodLog.ID),
		zap.String("food", foodLog.FoodName),
		zap.Int("calories", foodLog.EstimatedCalories),
	)

	summary, err := u.GetDailySummary(ctx, loggedAt)
	if err != nil {
		u.logger.Warn("failed to fetch daily summary after logging meal", zap.Error(err))
	}

	return foodLog, summary, nil
}

func (u *nutritionUsecase) GetDailySummary(ctx context.Context, date time.Time) (*domain.DailyNutritionSummary, error) {
	targetCalories := 2000
	if u.profileUC != nil {
		profile, err := u.profileUC.GetProfile(ctx)
		if err == nil && profile != nil && profile.DailyCalorieTarget > 0 {
			targetCalories = profile.DailyCalorieTarget
		}
	}

	summary, err := u.repo.GetDailySummary(ctx, date, targetCalories)
	if err != nil {
		return nil, fmt.Errorf("nutrition usecase get daily summary: %w", err)
	}

	// Buat saran otomatis berdasarkan sisa kalori
	if summary.Advice == "" {
		if summary.RemainingCalories < 0 {
			summary.Advice = fmt.Sprintf("⚠️ Asupan kalori hari ini sudah melewati target sebanyak %d kcal. Seimbangkan dengan olahraga ringan ya wir!", -summary.RemainingCalories)
		} else if summary.RemainingCalories < 300 {
			summary.Advice = fmt.Sprintf("💡 Sisa kuota kalori tinggal %d kcal lagi. Kalau mau ngemil, pilih buah atau snack rendah kalori ya!", summary.RemainingCalories)
		} else {
			summary.Advice = fmt.Sprintf("✅ Asupan kalori masih aman! Sisa kuota hari ini: %d kcal.", summary.RemainingCalories)
		}
	}

	return summary, nil
}

// FormatMealConfirmation formats logged meal information along with daily summary.
func FormatMealConfirmation(food *domain.FoodLog, summary *domain.DailyNutritionSummary) string {
	var sb strings.Builder
	sb.WriteString("🍽️ *Makanan Tercatat!*\n\n")
	sb.WriteString(fmt.Sprintf("• Menu: *%s*\n", food.FoodName))
	sb.WriteString(fmt.Sprintf("• Estimasi Kalori: *~%d kcal*\n", food.EstimatedCalories))
	if food.Protein > 0 || food.Carbs > 0 || food.Fat > 0 {
		sb.WriteString(fmt.Sprintf("• Makronutrisi: P: %.1fg | C: %.1fg | F: %.1fg\n", food.Protein, food.Carbs, food.Fat))
	}
	if food.Notes != "" {
		sb.WriteString(fmt.Sprintf("• Catatan: %s\n", food.Notes))
	}

	if summary != nil {
		sb.WriteString("\n📊 *Status Kalori Hari Ini:*\n")
		sb.WriteString(fmt.Sprintf("• Terkonsumsi: *%d kcal* / Target: *%d kcal*\n", summary.TotalCalories, summary.TargetCalories))
		if summary.RemainingCalories >= 0 {
			sb.WriteString(fmt.Sprintf("• Sisa Kuota: *%d kcal*\n", summary.RemainingCalories))
		} else {
			sb.WriteString(fmt.Sprintf("• Kelebihan: *+%d kcal* ⚠️\n", -summary.RemainingCalories))
		}
		if summary.Advice != "" {
			sb.WriteString(fmt.Sprintf("\n%s", summary.Advice))
		}
	}

	return sb.String()
}
