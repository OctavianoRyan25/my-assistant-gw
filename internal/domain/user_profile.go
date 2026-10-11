package domain

import (
	"context"
	"time"
)

// Gender constants
const (
	GenderMale   = "male"
	GenderFemale = "female"
)

// Activity level constants (used for TDEE calculation)
const (
	ActivitySedentary        = "sedentary"          // Tidak banyak bergerak / kerja kantoran
	ActivityLightlyActive    = "lightly_active"     // Olahraga ringan 1-3 hari/minggu
	ActivityModeratelyActive = "moderately_active"  // Olahraga sedang 3-5 hari/minggu
	ActivityVeryActive       = "very_active"        // Olahraga berat 6-7 hari/minggu
	ActivityExtraActive      = "extra_active"       // Pekerjaan fisik berat / atlet latihan 2x sehari
)

// UserProfile represents user physical metrics (TB & BB) and base settings.
type UserProfile struct {
	ID                 int64      `json:"id"`
	HeightCM           float64    `json:"height_cm"`             // Tinggi badan dalam cm (TB)
	WeightKG           float64    `json:"weight_kg"`             // Berat badan dalam kg (BB)
	Age                int        `json:"age"`                   // Usia pengguna (opsional untuk BMR)
	Gender             string     `json:"gender"`                // male / female
	ActivityLevel      string     `json:"activity_level"`        // sedentary, lightly_active, etc.
	DailyCalorieTarget int        `json:"daily_calorie_target"`  // Target kalori harian (dihitung dari TDEE atau di-set manual)
	MonthlySalary      float64    `json:"monthly_salary"`        // Gaji per bulan (digunakan sebagai baseline threshold budget)
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          *time.Time `json:"updated_at,omitempty"`
}

// UserProfileRepository defines persistence operations for user profile.
type UserProfileRepository interface {
	Get(ctx context.Context) (*UserProfile, error)
	Save(ctx context.Context, profile *UserProfile) error
}

// UserProfileUsecase defines business logic for managing user profile & calculating physical targets.
type UserProfileUsecase interface {
	GetProfile(ctx context.Context) (*UserProfile, error)
	UpdatePhysicalMetrics(ctx context.Context, heightCM, weightKG float64, age int, gender, activityLevel string) (*UserProfile, error)
	SetSalary(ctx context.Context, monthlySalary float64) (*UserProfile, error)
	CalculateBMRAndTDEE(profile *UserProfile) (bmr float64, tdee float64)
}
