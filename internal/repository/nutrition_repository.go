package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
)

type nutritionRepository struct {
	db *sql.DB
}

// NewNutritionRepository creates a new NutritionRepository.
func NewNutritionRepository(db *sql.DB) domain.NutritionRepository {
	return &nutritionRepository{db: db}
}

func (r *nutritionRepository) Create(ctx context.Context, log *domain.FoodLog) error {
	query := `
		INSERT INTO nutrition_logs (food_name, estimated_calories, protein, carbs, fat, confidence, notes, logged_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	result, err := r.db.ExecContext(ctx, query,
		log.FoodName,
		log.EstimatedCalories,
		log.Protein,
		log.Carbs,
		log.Fat,
		log.Confidence,
		log.Notes,
		log.LoggedAt,
		log.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("nutrition repository create: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("nutrition repository get insert id: %w", err)
	}
	log.ID = id
	return nil
}

func (r *nutritionRepository) GetByID(ctx context.Context, id int64) (*domain.FoodLog, error) {
	query := `
		SELECT id, food_name, estimated_calories, protein, carbs, fat, confidence, notes, logged_at, created_at
		FROM nutrition_logs
		WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, query, id)
	return scanFoodLog(row)
}

func (r *nutritionRepository) GetByDateRange(ctx context.Context, start, end time.Time) ([]*domain.FoodLog, error) {
	query := `
		SELECT id, food_name, estimated_calories, protein, carbs, fat, confidence, notes, logged_at, created_at
		FROM nutrition_logs
		WHERE logged_at >= ? AND logged_at <= ?
		ORDER BY logged_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, start, end)
	if err != nil {
		return nil, fmt.Errorf("nutrition repository get by date range: %w", err)
	}
	defer rows.Close()

	var logs []*domain.FoodLog
	for rows.Next() {
		var l domain.FoodLog
		var notes sql.NullString
		if err := rows.Scan(
			&l.ID,
			&l.FoodName,
			&l.EstimatedCalories,
			&l.Protein,
			&l.Carbs,
			&l.Fat,
			&l.Confidence,
			&notes,
			&l.LoggedAt,
			&l.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("nutrition repository scan row: %w", err)
		}
		if notes.Valid {
			l.Notes = notes.String
		}
		logs = append(logs, &l)
	}
	return logs, rows.Err()
}

func (r *nutritionRepository) GetDailySummary(ctx context.Context, date time.Time, targetCalories int) (*domain.DailyNutritionSummary, error) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	end := start.Add(24 * time.Hour).Add(-time.Second)

	queryAgg := `
		SELECT
			COALESCE(SUM(estimated_calories), 0),
			COALESCE(SUM(protein), 0),
			COALESCE(SUM(carbs), 0),
			COALESCE(SUM(fat), 0)
		FROM nutrition_logs
		WHERE logged_at >= ? AND logged_at <= ?
	`
	var totalCalories int
	var totalProtein, totalCarbs, totalFat float64

	err := r.db.QueryRowContext(ctx, queryAgg, start, end).Scan(
		&totalCalories,
		&totalProtein,
		&totalCarbs,
		&totalFat,
	)
	if err != nil {
		return nil, fmt.Errorf("nutrition repository get daily aggregates: %w", err)
	}

	meals, err := r.GetByDateRange(ctx, start, end)
	if err != nil {
		return nil, err
	}

	remaining := targetCalories - totalCalories
	status := domain.CalorieStatusOnTrack
	if targetCalories > 0 {
		if totalCalories > targetCalories {
			status = domain.CalorieStatusOverTarget
		} else if remaining > 500 {
			status = domain.CalorieStatusUnderTarget
		}
	}

	return &domain.DailyNutritionSummary{
		Date:              start,
		TargetCalories:    targetCalories,
		TotalCalories:     totalCalories,
		RemainingCalories: remaining,
		Status:            status,
		TotalProtein:      totalProtein,
		TotalCarbs:        totalCarbs,
		TotalFat:          totalFat,
		Meals:             meals,
	}, nil
}

func (r *nutritionRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM nutrition_logs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("nutrition repository delete: %w", err)
	}
	return nil
}

func scanFoodLog(row *sql.Row) (*domain.FoodLog, error) {
	var l domain.FoodLog
	var notes sql.NullString
	err := row.Scan(
		&l.ID,
		&l.FoodName,
		&l.EstimatedCalories,
		&l.Protein,
		&l.Carbs,
		&l.Fat,
		&l.Confidence,
		&notes,
		&l.LoggedAt,
		&l.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan food log: %w", err)
	}
	if notes.Valid {
		l.Notes = notes.String
	}
	return &l, nil
}
