package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
)

type memoryRepository struct {
	db *sql.DB
}

// NewMemoryRepository creates a new MySQL-backed MemoryRepository.
// It automatically ensures the user_memories table exists.
func NewMemoryRepository(db *sql.DB) domain.MemoryRepository {
	repo := &memoryRepository{db: db}
	repo.ensureTableExists()
	return repo
}

func (r *memoryRepository) ensureTableExists() {
	query := `
		CREATE TABLE IF NOT EXISTS user_memories (
			id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			category   VARCHAR(50)     NOT NULL DEFAULT 'personal',
			fact       TEXT            NOT NULL,
			created_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			INDEX idx_user_memories_category (category),
			INDEX idx_user_memories_created_at (created_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
	`
	_, _ = r.db.Exec(query)
}

func (r *memoryRepository) Save(ctx context.Context, m *domain.Memory) error {
	now := time.Now()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now

	query := `INSERT INTO user_memories (category, fact, created_at, updated_at) VALUES (?, ?, ?, ?)`
	res, err := r.db.ExecContext(ctx, query, m.Category, m.Fact, m.CreatedAt, m.UpdatedAt)
	if err != nil {
		return fmt.Errorf("memory repo save: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("memory repo get last insert id: %w", err)
	}
	m.ID = id
	return nil
}

func (r *memoryRepository) GetAll(ctx context.Context) ([]*domain.Memory, error) {
	query := `SELECT id, category, fact, created_at, updated_at FROM user_memories ORDER BY updated_at DESC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("memory repo get all: %w", err)
	}
	defer rows.Close()

	var memories []*domain.Memory
	for rows.Next() {
		var m domain.Memory
		if err := rows.Scan(&m.ID, &m.Category, &m.Fact, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("memory repo scan: %w", err)
		}
		memories = append(memories, &m)
	}
	return memories, rows.Err()
}

func (r *memoryRepository) GetByCategory(ctx context.Context, category string) ([]*domain.Memory, error) {
	query := `SELECT id, category, fact, created_at, updated_at FROM user_memories WHERE category = ? ORDER BY updated_at DESC`
	rows, err := r.db.QueryContext(ctx, query, category)
	if err != nil {
		return nil, fmt.Errorf("memory repo get by category: %w", err)
	}
	defer rows.Close()

	var memories []*domain.Memory
	for rows.Next() {
		var m domain.Memory
		if err := rows.Scan(&m.ID, &m.Category, &m.Fact, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("memory repo scan: %w", err)
		}
		memories = append(memories, &m)
	}
	return memories, rows.Err()
}

func (r *memoryRepository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM user_memories WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("memory repo delete: %w", err)
	}
	return nil
}

// FindSimilar checks if a fact or very similar wording already exists in memory.
func (r *memoryRepository) FindSimilar(ctx context.Context, fact string) (bool, error) {
	clean := strings.TrimSpace(fact)
	if clean == "" {
		return true, nil
	}

	query := `SELECT COUNT(*) FROM user_memories WHERE fact = ? OR LOWER(fact) = LOWER(?) LIMIT 1`
	var count int
	err := r.db.QueryRowContext(ctx, query, clean, clean).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
