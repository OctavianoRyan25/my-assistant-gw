package domain

import (
	"context"
	"time"
)

// Workout type constants
const (
	WorkoutTypeGym      = "gym"
	WorkoutTypeJogging  = "jogging"
	WorkoutTypeRunning  = "running"
	WorkoutTypeSwimming = "swimming"
	WorkoutTypeCycling  = "cycling"
	WorkoutTypeWalking  = "walking"
	WorkoutTypeHIIT     = "hiit"
	WorkoutTypeOther    = "other"
)

// Workout represents a completed workout activity log.
type Workout struct {
	ID              int64      `json:"id"`
	Title           string     `json:"title"`            // e.g. "Gym Push Day", "Jogging Pagi di GBK"
	Type            string     `json:"type"`             // gym, jogging, running, swimming, etc.
	DurationMinutes int        `json:"duration_minutes"` // durasi aktivitas dalam menit
	CaloriesBurned  int        `json:"calories_burned"`  // estimasi kalori yang terbakar
	DistanceMeters  float64    `json:"distance_meters"`  // jarak tempuh (opsional, untuk lari/jogging/renang/sepeda)
	Description     string     `json:"description"`      // catatan latihan (e.g. "Bench press 4x8, shoulder press 3x10")
	LoggedAt        time.Time  `json:"logged_at"`        // waktu pelaksanaan workout
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

// WorkoutMonthlyStats aggregates monthly workout activities.
type WorkoutMonthlyStats struct {
	Period               string         `json:"period"`                 // e.g. "Oktober 2026"
	Year                 int            `json:"year"`
	Month                int            `json:"month"`
	TotalSessions        int            `json:"total_sessions"`         // jumlah kali berolahraga
	TotalDurationMinutes int            `json:"total_duration_minutes"` // total menit olahraga
	TotalCaloriesBurned  int            `json:"total_calories_burned"`  // total kalori terbakar
	TypeBreakdown        map[string]int `json:"type_breakdown"`         // jumlah sesi per tipe (gym: 8, jogging: 4)
	RecentWorkouts       []*Workout     `json:"recent_workouts"`
}

// WorkoutParsed is the structured workout extracted from natural language or image.
type WorkoutParsed struct {
	Title           string  `json:"title"`
	Type            string  `json:"type"`
	DurationMinutes int     `json:"duration_minutes"`
	CaloriesBurned  int     `json:"calories_burned"`
	DistanceMeters  float64 `json:"distance_meters"`
	Description     string  `json:"description"`
}

// WorkoutRepository defines persistence operations for workout logs.
type WorkoutRepository interface {
	Create(ctx context.Context, workout *Workout) error
	GetByID(ctx context.Context, id int64) (*Workout, error)
	GetByDateRange(ctx context.Context, start, end time.Time) ([]*Workout, error)
	GetMonthlyStats(ctx context.Context, year, month int) (*WorkoutMonthlyStats, error)
	Delete(ctx context.Context, id int64) error
}

// WorkoutUsecase defines business logic for workout tracking and monthly stats.
type WorkoutUsecase interface {
	RecordWorkout(ctx context.Context, workout *Workout) (*Workout, error)
	RecordWorkoutFromText(ctx context.Context, message string) (*Workout, error)
	RecordWorkoutFromImage(ctx context.Context, imageData []byte, mimeType, caption string) (*Workout, error)
	GetMonthlyStats(ctx context.Context, year, month int) (*WorkoutMonthlyStats, error)
	GetCurrentMonthStats(ctx context.Context) (*WorkoutMonthlyStats, error)
	SendMonthlyReportIfDue(ctx context.Context) error
}

// WorkoutParser defines the LLM contract for extracting workout data from text or image.
type WorkoutParser interface {
	ParseWorkoutFromText(ctx context.Context, message string) (*WorkoutParsed, error)
	ParseWorkoutFromImage(ctx context.Context, imageData []byte, mimeType, caption string) (*WorkoutParsed, error)
}
