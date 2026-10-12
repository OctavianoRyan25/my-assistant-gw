package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
)

type workoutRepository struct {
	db *sql.DB
}

// NewWorkoutRepository creates a new MySQL/MariaDB-backed WorkoutRepository.
func NewWorkoutRepository(db *sql.DB) domain.WorkoutRepository {
	return &workoutRepository{db: db}
}

func (r *workoutRepository) Create(ctx context.Context, workout *domain.Workout) error {
	query := `
		INSERT INTO workouts (title, type, duration_minutes, calories_burned, distance_meters, description, logged_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	result, err := r.db.ExecContext(ctx, query,
		workout.Title,
		workout.Type,
		workout.DurationMinutes,
		workout.CaloriesBurned,
		workout.DistanceMeters,
		workout.Description,
		workout.LoggedAt,
		workout.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("workout repository create: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("workout repository get last insert id: %w", err)
	}
	workout.ID = id
	return nil
}

func (r *workoutRepository) GetByID(ctx context.Context, id int64) (*domain.Workout, error) {
	query := `
		SELECT id, title, type, duration_minutes, calories_burned, distance_meters, description, logged_at, created_at, updated_at
		FROM workouts
		WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, query, id)
	return scanWorkout(row)
}

func (r *workoutRepository) GetByDateRange(ctx context.Context, start, end time.Time) ([]*domain.Workout, error) {
	query := `
		SELECT id, title, type, duration_minutes, calories_burned, distance_meters, description, logged_at, created_at, updated_at
		FROM workouts
		WHERE logged_at >= ? AND logged_at <= ?
		ORDER BY logged_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, start, end)
	if err != nil {
		return nil, fmt.Errorf("workout repository get by date range: %w", err)
	}
	defer rows.Close()

	var workouts []*domain.Workout
	for rows.Next() {
		var w domain.Workout
		var desc sql.NullString
		var updatedAt sql.NullTime
		if err := rows.Scan(
			&w.ID,
			&w.Title,
			&w.Type,
			&w.DurationMinutes,
			&w.CaloriesBurned,
			&w.DistanceMeters,
			&desc,
			&w.LoggedAt,
			&w.CreatedAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("workout repository scan row: %w", err)
		}
		if desc.Valid {
			w.Description = desc.String
		}
		if updatedAt.Valid {
			w.UpdatedAt = &updatedAt.Time
		}
		workouts = append(workouts, &w)
	}
	return workouts, rows.Err()
}

func (r *workoutRepository) GetMonthlyStats(ctx context.Context, year, month int) (*domain.WorkoutMonthlyStats, error) {
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	end := start.AddDate(0, 1, 0).Add(-time.Second)

	// Agregasi total sesi, total menit, total kalori
	var totalSessions int
	var totalDuration int
	var totalCalories int

	queryAgg := `
		SELECT
			COALESCE(COUNT(id), 0),
			COALESCE(SUM(duration_minutes), 0),
			COALESCE(SUM(calories_burned), 0)
		FROM workouts
		WHERE logged_at >= ? AND logged_at <= ?
	`
	err := r.db.QueryRowContext(ctx, queryAgg, start, end).Scan(&totalSessions, &totalDuration, &totalCalories)
	if err != nil {
		return nil, fmt.Errorf("workout repository get monthly totals: %w", err)
	}

	// Breakdown per kategori/type olahraga
	queryBreakdown := `
		SELECT type, COUNT(id)
		FROM workouts
		WHERE logged_at >= ? AND logged_at <= ?
		GROUP BY type
		ORDER BY COUNT(id) DESC
	`
	rows, err := r.db.QueryContext(ctx, queryBreakdown, start, end)
	if err != nil {
		return nil, fmt.Errorf("workout repository get type breakdown: %w", err)
	}
	defer rows.Close()

	typeBreakdown := make(map[string]int)
	for rows.Next() {
		var wType string
		var count int
		if err := rows.Scan(&wType, &count); err != nil {
			return nil, fmt.Errorf("workout repository scan breakdown row: %w", err)
		}
		typeBreakdown[wType] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Ambil 5 aktivitas terbaru bulan ini
	recentWorkouts, err := r.GetByDateRange(ctx, start, end)
	if err != nil {
		return nil, err
	}
	if len(recentWorkouts) > 5 {
		recentWorkouts = recentWorkouts[:5]
	}

	return &domain.WorkoutMonthlyStats{
		Period:               fmt.Sprintf("%d-%02d", year, month),
		Year:                 year,
		Month:                month,
		TotalSessions:        totalSessions,
		TotalDurationMinutes: totalDuration,
		TotalCaloriesBurned:  totalCalories,
		TypeBreakdown:        typeBreakdown,
		RecentWorkouts:       recentWorkouts,
	}, nil
}

func (r *workoutRepository) GetRecent(ctx context.Context, limit int) ([]*domain.Workout, error) {
	if limit <= 0 {
		limit = 10
	}
	query := `
		SELECT id, title, type, duration_minutes, calories_burned, distance_meters, description, logged_at, created_at, updated_at
		FROM workouts
		ORDER BY logged_at DESC, id DESC
		LIMIT ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("workout repository get recent: %w", err)
	}
	defer rows.Close()

	var workouts []*domain.Workout
	for rows.Next() {
		var w domain.Workout
		var desc sql.NullString
		var updatedAt sql.NullTime
		if err := rows.Scan(
			&w.ID,
			&w.Title,
			&w.Type,
			&w.DurationMinutes,
			&w.CaloriesBurned,
			&w.DistanceMeters,
			&desc,
			&w.LoggedAt,
			&w.CreatedAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("workout repository scan row: %w", err)
		}
		if desc.Valid {
			w.Description = desc.String
		}
		if updatedAt.Valid {
			w.UpdatedAt = &updatedAt.Time
		}
		workouts = append(workouts, &w)
	}
	return workouts, rows.Err()
}

func (r *workoutRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM workouts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("workout repository delete: %w", err)
	}
	return nil
}

func scanWorkout(row *sql.Row) (*domain.Workout, error) {
	var w domain.Workout
	var desc sql.NullString
	var updatedAt sql.NullTime

	err := row.Scan(
		&w.ID,
		&w.Title,
		&w.Type,
		&w.DurationMinutes,
		&w.CaloriesBurned,
		&w.DistanceMeters,
		&desc,
		&w.LoggedAt,
		&w.CreatedAt,
		&updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan workout: %w", err)
	}
	if desc.Valid {
		w.Description = desc.String
	}
	if updatedAt.Valid {
		w.UpdatedAt = &updatedAt.Time
	}
	return &w, nil
}
