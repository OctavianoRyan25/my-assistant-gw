package domain

import (
	"context"
	"time"
)

// Chat roles
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
)

// Intent types for routing
const (
	IntentReminder    = "reminder"
	IntentExpense     = "expense"
	IntentSearch      = "search"
	IntentGeneralChat = "general_chat"
	IntentReset       = "reset"
)

// ChatMessage represents a single turn in the conversation.
type ChatMessage struct {
	ID        int64     `json:"id"`
	Role      string    `json:"role"` // user/assistant/system
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// IntentResult holds the classified intent from a user message.
type IntentResult struct {
	Intent     string  `json:"intent"`
	Confidence float64 `json:"confidence"`
	RawMessage string  `json:"raw_message"`
}

// ChatRepository defines persistence operations for chat history.
type ChatRepository interface {
	SaveMessage(ctx context.Context, msg *ChatMessage) error
	GetRecentMessages(ctx context.Context, limit int) ([]*ChatMessage, error)
	ClearHistory(ctx context.Context) error
}

// ChatUsecase defines business logic for general chat & orchestration.
type ChatUsecase interface {
	HandleMessage(ctx context.Context, userMessage string) (string, error)
	ResetContext(ctx context.Context) error
	GetHistory(ctx context.Context, limit int) ([]*ChatMessage, error)
}

// LLMClient defines the contract for any LLM provider.
type LLMClient interface {
	// Chat sends a list of messages and returns the assistant reply.
	Chat(ctx context.Context, messages []ChatMessage) (string, error)
	// ClassifyIntent uses a lightweight call to determine user intent.
	ClassifyIntent(ctx context.Context, message string) (*IntentResult, error)
	// ParseReminder extracts a single reminder from natural language.
	// Kept for backward compatibility; internally delegates to ParseReminders.
	ParseReminder(ctx context.Context, message string) (title string, scheduledAt string, recurrence *string, err error)
	// ParseReminders extracts one or more reminders from natural language.
	// Handles both single objects and arrays returned by the LLM.
	ParseReminders(ctx context.Context, message string) ([]ReminderParsed, error)
	// ParseExpense extracts structured expense data from natural language.
	ParseExpense(ctx context.Context, message string) (amount float64, category string, description string, err error)
}

// ReminderParsed is the structured data extracted from a natural language reminder message.
type ReminderParsed struct {
	Title       string  `json:"title"`
	ScheduledAt string  `json:"scheduled_at"`
	Recurrence  *string `json:"recurrence"`
}

// SearchClient defines the contract for web search providers.
type SearchClient interface {
	Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error)
}

// SearchResult holds a single web search result.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// WhatsAppSender defines the contract for sending WhatsApp messages.
type WhatsAppSender interface {
	SendMessage(ctx context.Context, to, message string) error
}
