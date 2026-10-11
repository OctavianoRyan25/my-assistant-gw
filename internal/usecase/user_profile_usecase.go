package usecase

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
	"go.uber.org/zap"
)

type userProfileUsecase struct {
	repo   domain.UserProfileRepository
	logger *zap.Logger
}

// NewUserProfileUsecase creates a new UserProfileUsecase.
func NewUserProfileUsecase(
	repo domain.UserProfileRepository,
	logger *zap.Logger,
) domain.UserProfileUsecase {
	return &userProfileUsecase{
		repo:   repo,
		logger: logger,
	}
}

func (u *userProfileUsecase) GetProfile(ctx context.Context) (*domain.UserProfile, error) {
	profile, err := u.repo.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("user profile usecase get: %w", err)
	}
	if profile == nil {
		// Nilai default jika belum pernah diset
		profile = &domain.UserProfile{
			HeightCM:           170,
			WeightKG:           65,
			Age:                25,
			Gender:             domain.GenderMale,
			ActivityLevel:      domain.ActivityModeratelyActive,
			DailyCalorieTarget: 2100,
			MonthlySalary:      0,
		}
	}
	return profile, nil
}

func (u *userProfileUsecase) UpdatePhysicalMetrics(
	ctx context.Context,
	heightCM, weightKG float64,
	age int,
	gender, activityLevel string,
) (*domain.UserProfile, error) {
	profile, err := u.GetProfile(ctx)
	if err != nil {
		return nil, err
	}

	if heightCM > 0 {
		profile.HeightCM = heightCM
	}
	if weightKG > 0 {
		profile.WeightKG = weightKG
	}
	if age > 0 {
		profile.Age = age
	}
	if gender != "" {
		profile.Gender = strings.ToLower(gender)
	}
	if activityLevel != "" {
		profile.ActivityLevel = strings.ToLower(activityLevel)
	}

	// Hitung ulang target kalori berdasarkan rumus Mifflin-St Jeor
	_, tdee := u.CalculateBMRAndTDEE(profile)
	profile.DailyCalorieTarget = int(math.Round(tdee))

	if err := u.repo.Save(ctx, profile); err != nil {
		return nil, fmt.Errorf("save updated physical metrics: %w", err)
	}
	u.logger.Info("user physical profile updated",
		zap.Float64("height", profile.HeightCM),
		zap.Float64("weight", profile.WeightKG),
		zap.Int("daily_target", profile.DailyCalorieTarget),
	)
	return profile, nil
}

func (u *userProfileUsecase) SetSalary(ctx context.Context, monthlySalary float64) (*domain.UserProfile, error) {
	profile, err := u.GetProfile(ctx)
	if err != nil {
		return nil, err
	}

	profile.MonthlySalary = monthlySalary
	if err := u.repo.Save(ctx, profile); err != nil {
		return nil, fmt.Errorf("save updated monthly salary: %w", err)
	}
	u.logger.Info("user monthly salary updated", zap.Float64("salary", monthlySalary))
	return profile, nil
}

// CalculateBMRAndTDEE calculates Basal Metabolic Rate and Total Daily Energy Expenditure.
// Menggunakan formula Mifflin-St Jeor.
func (u *userProfileUsecase) CalculateBMRAndTDEE(profile *domain.UserProfile) (bmr float64, tdee float64) {
	if profile == nil {
		return 0, 0
	}

	age := profile.Age
	if age <= 0 {
		age = 25
	}

	// BMR = (10 * BB) + (6.25 * TB) - (5 * umur) + s
	// s = +5 untuk pria, -161 untuk wanita
	s := 5.0
	if strings.ToLower(profile.Gender) == domain.GenderFemale {
		s = -161.0
	}
	bmr = (10.0 * profile.WeightKG) + (6.25 * profile.HeightCM) - (5.0 * float64(age)) + s
	if bmr < 0 {
		bmr = 1500
	}

	// Multiplier aktivitas
	multiplier := 1.55 // default moderately active
	switch strings.ToLower(profile.ActivityLevel) {
	case domain.ActivitySedentary:
		multiplier = 1.2
	case domain.ActivityLightlyActive:
		multiplier = 1.375
	case domain.ActivityModeratelyActive:
		multiplier = 1.55
	case domain.ActivityVeryActive:
		multiplier = 1.725
	case domain.ActivityExtraActive:
		multiplier = 1.9
	}

	tdee = bmr * multiplier
	return bmr, tdee
}

// FormatProfileInfo formats profile data into a readable WA message.
func FormatProfileInfo(profile *domain.UserProfile, bmr, tdee float64) string {
	bmi := 0.0
	bmiCategory := "Normal"
	if profile.HeightCM > 0 {
		hMeter := profile.HeightCM / 100.0
		bmi = profile.WeightKG / (hMeter * hMeter)
		if bmi < 18.5 {
			bmiCategory = "Underweight (Kurus)"
		} else if bmi >= 25.0 && bmi < 29.9 {
			bmiCategory = "Overweight (Kelebihan)"
		} else if bmi >= 30.0 {
			bmiCategory = "Obesitas"
		}
	}

	var sb strings.Builder
	sb.WriteString("👤 *Profil Kesehatan Pengguna*\n\n")
	sb.WriteString(fmt.Sprintf("• Tinggi Badan (TB): *%.1f cm*\n", profile.HeightCM))
	sb.WriteString(fmt.Sprintf("• Berat Badan (BB): *%.1f kg*\n", profile.WeightKG))
	sb.WriteString(fmt.Sprintf("• BMI: *%.1f* (%s)\n", bmi, bmiCategory))
	sb.WriteString(fmt.Sprintf("• BMR (Metabolisme Dasar): *%.0f kcal*\n", bmr))
	sb.WriteString(fmt.Sprintf("• TDEE (Kebutuhan Harian): *%.0f kcal*\n", tdee))
	sb.WriteString(fmt.Sprintf("• Target Kalori Harian: *%d kcal*\n", profile.DailyCalorieTarget))
	if profile.MonthlySalary > 0 {
		sb.WriteString(fmt.Sprintf("• Baseline Gaji Bulanan: *Rp %s*\n", formatRupiah(profile.MonthlySalary)))
	}
	return sb.String()
}
