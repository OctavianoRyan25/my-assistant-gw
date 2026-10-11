package domain

import (
	"context"
	"time"
)

// Memory category constants
const (
	MemoryCategoryPersonality = "personality" // Sifat, kepribadian, gaya komunikasi
	MemoryCategoryPreference  = "preference"  // Preferensi makanan, minuman, tools, musik, dsb
	MemoryCategoryHabit       = "habit"       // Kebiasaan sehari-hari, jam tidur, olahraga
	MemoryCategoryWork        = "work"        // Pekerjaan, tech stack, proyek, tanggung jawab
	MemoryCategoryPersonal    = "personal"    // Data pribadi, keluarga, teman, domisili, ulang tahun
)

// Memory represents a stored long-term fact or trait about the user.
type Memory struct {
	ID        int64     `json:"id"`
	Category  string    `json:"category"`
	Fact      string    `json:"fact"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MemoryItem is the lightweight extracted item returned by LLM.
type MemoryItem struct {
	Category string `json:"category"`
	Fact     string `json:"fact"`
}

// MemoryRepository defines persistence operations for user memories.
type MemoryRepository interface {
	Save(ctx context.Context, memory *Memory) error
	GetAll(ctx context.Context) ([]*Memory, error)
	GetByCategory(ctx context.Context, category string) ([]*Memory, error)
	Delete(ctx context.Context, id int64) error
	FindSimilar(ctx context.Context, fact string) (bool, error)
}
