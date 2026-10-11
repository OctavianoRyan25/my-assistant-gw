package domain

import (
	"context"
	"time"
)

// Calorie status constants
const (
	CalorieStatusUnderTarget = "under_target"
	CalorieStatusOnTrack     = "on_track"
	CalorieStatusOverTarget  = "over_target"
)

// FoodLog represents an individual meal or food calorie record.
type FoodLog struct {
	ID                int64     `json:"id"`
	FoodName          string    `json:"food_name"`          // e.g. "Nasi Padang Rendang + Telur Dadar"
	EstimatedCalories int       `json:"estimated_calories"` // Total kalori yang diestimasi
	Protein           float64   `json:"protein"`            // gram (opsional)
	Carbs             float64   `json:"carbs"`              // gram (opsional)
	Fat               float64   `json:"fat"`                // gram (opsional)
	Confidence        float64   `json:"confidence"`         // confidence level estimasi model AI
	Notes             string    `json:"notes"`              // keterangan porsi atau catatan tambahan
	LoggedAt          time.Time `json:"logged_at"`          // waktu makan
	CreatedAt         time.Time `json:"created_at"`
}

// DailyNutritionSummary aggregates calories and macros consumed on a specific day compared to the user target.
type DailyNutritionSummary struct {
	Date              time.Time  `json:"date"`
	TargetCalories    int        `json:"target_calories"`    // disesuaikan dari profil fisik (TB & BB)
	TotalCalories     int        `json:"total_calories"`     // total kalori hari ini
	RemainingCalories int        `json:"remaining_calories"` // sisa kalori yang bisa dikonsumsi
	Status            string     `json:"status"`             // under_target, on_track, over_target
	TotalProtein      float64    `json:"total_protein"`
	TotalCarbs        float64    `json:"total_carbs"`
	TotalFat          float64    `json:"total_fat"`
	Meals             []*FoodLog `json:"meals"`
	Advice            string     `json:"advice"`             // saran AI berdasarkan sisa kuota kalori
}

// MealParsed holds the structured food data extracted from an image via Vision AI.
type MealParsed struct {
	FoodName           string  `json:"food_name"`
	EstimatedCalories  int     `json:"estimated_calories"`
	Protein            float64 `json:"protein"`
	Carbs              float64 `json:"carbs"`
	Fat                float64 `json:"fat"`
	PortionDescription string  `json:"portion_description"`
	Confidence         float64 `json:"confidence"`
	Advice             string  `json:"advice"`
}

// NutritionRepository defines persistence operations for food and calorie logs.
type NutritionRepository interface {
	Create(ctx context.Context, log *FoodLog) error
	GetByID(ctx context.Context, id int64) (*FoodLog, error)
	GetByDateRange(ctx context.Context, start, end time.Time) ([]*FoodLog, error)
	GetDailySummary(ctx context.Context, date time.Time, targetCalories int) (*DailyNutritionSummary, error)
	Delete(ctx context.Context, id int64) error
}

// NutritionUsecase defines business logic for logging meals and comparing against TB/BB targets.
type NutritionUsecase interface {
	LogMealFromImage(ctx context.Context, imageData []byte, mimeType string, notes string) (*FoodLog, *DailyNutritionSummary, error)
	LogMealFromText(ctx context.Context, foodName string, calories int, notes string, loggedAt time.Time) (*FoodLog, *DailyNutritionSummary, error)
	GetDailySummary(ctx context.Context, date time.Time) (*DailyNutritionSummary, error)
}

// NutritionParser defines the contract for vision-based meal calorie recognition.
type NutritionParser interface {
	ParseMealFromImage(ctx context.Context, imageData []byte, mimeType string) (*MealParsed, error)
}
