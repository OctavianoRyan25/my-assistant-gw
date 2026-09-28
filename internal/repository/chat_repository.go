package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
)

type chatRepository struct {
	db *sql.DB
}

// NewChatRepository creates a new MySQL-backed ChatRepository.
func NewChatRepository(db *sql.DB) domain.ChatRepository {
	return &chatRepository{db: db}
}

func (r *chatRepository) SaveMessage(ctx context.Context, msg *domain.ChatMessage) error {
	query := `INSERT INTO chat_history (role, message, created_at) VALUES (?, ?, ?)`
	result, err := r.db.ExecContext(ctx, query, msg.Role, msg.Message, msg.CreatedAt)
	if err != nil {
		return fmt.Errorf("chat repository save message: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("chat repository get last insert id: %w", err)
	}
	msg.ID = id
	return nil
}

func (r *chatRepository) GetRecentMessages(ctx context.Context, limit int) ([]*domain.ChatMessage, error) {
	// Get last N messages ordered ascending so LLM sees them in chronological order
	query := `
		SELECT id, role, message, created_at FROM (
			SELECT id, role, message, created_at FROM chat_history
			ORDER BY created_at DESC
			LIMIT ?
		) sub
		ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("chat repository get recent messages: %w", err)
	}
	defer rows.Close()

	var messages []*domain.ChatMessage
	for rows.Next() {
		var msg domain.ChatMessage
		if err := rows.Scan(&msg.ID, &msg.Role, &msg.Message, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("chat repository scan message: %w", err)
		}
		messages = append(messages, &msg)
	}
	return messages, rows.Err()
}

func (r *chatRepository) ClearHistory(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `TRUNCATE TABLE chat_history`)
	if err != nil {
		return fmt.Errorf("chat repository clear history: %w", err)
	}
	return nil
}
