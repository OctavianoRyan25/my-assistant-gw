package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
)

type userProfileRepository struct {
	db *sql.DB
}

// NewUserProfileRepository creates a new UserProfileRepository.
func NewUserProfileRepository(db *sql.DB) domain.UserProfileRepository {
	return &userProfileRepository{db: db}
}

func (r *userProfileRepository) Get(ctx context.Context) (*domain.UserProfile, error) {
	query := `
		SELECT id, height_cm, weight_kg, age, gender, activity_level, daily_calorie_target, monthly_salary, created_at, updated_at
		FROM user_profiles
		ORDER BY id ASC
		LIMIT 1
	`
	row := r.db.QueryRowContext(ctx, query)

	var p domain.UserProfile
	var updatedAt sql.NullTime
	err := row.Scan(
		&p.ID,
		&p.HeightCM,
		&p.WeightKG,
		&p.Age,
		&p.Gender,
		&p.ActivityLevel,
		&p.DailyCalorieTarget,
		&p.MonthlySalary,
		&p.CreatedAt,
		&updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("user profile repository get: %w", err)
	}
	if updatedAt.Valid {
		p.UpdatedAt = &updatedAt.Time
	}
	return &p, nil
}

func (r *userProfileRepository) Save(ctx context.Context, profile *domain.UserProfile) error {
	existing, err := r.Get(ctx)
	if err != nil {
		return err
	}

	now := time.Now()
	if existing == nil {
		query := `
			INSERT INTO user_profiles (height_cm, weight_kg, age, gender, activity_level, daily_calorie_target, monthly_salary, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`
		result, err := r.db.ExecContext(ctx, query,
			profile.HeightCM,
			profile.WeightKG,
			profile.Age,
			profile.Gender,
			profile.ActivityLevel,
			profile.DailyCalorieTarget,
			profile.MonthlySalary,
			now,
		)
		if err != nil {
			return fmt.Errorf("user profile repository insert: %w", err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("user profile repository get insert id: %w", err)
		}
		profile.ID = id
		profile.CreatedAt = now
		return nil
	}

	query := `
		UPDATE user_profiles
		SET height_cm = ?, weight_kg = ?, age = ?, gender = ?, activity_level = ?, daily_calorie_target = ?, monthly_salary = ?, updated_at = ?
		WHERE id = ?
	`
	_, err = r.db.ExecContext(ctx, query,
		profile.HeightCM,
		profile.WeightKG,
		profile.Age,
		profile.Gender,
		profile.ActivityLevel,
		profile.DailyCalorieTarget,
		profile.MonthlySalary,
		now,
		existing.ID,
	)
	if err != nil {
		return fmt.Errorf("user profile repository update: %w", err)
	}
	profile.ID = existing.ID
	profile.UpdatedAt = &now
	return nil
}
