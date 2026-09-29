package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
	"go.uber.org/zap"
)

const (
	// maxChatHistory is the number of recent messages included in LLM context.
	maxChatHistory = 20
	// maxSearchResults limits how many search results are passed as context.
	maxSearchResults = 5
)

type chatUsecase struct {
	chatRepo     domain.ChatRepository
	llm          domain.LLMClient
	searchClient domain.SearchClient
	reminderUC   domain.ReminderUsecase
	expenseUC    domain.ExpenseUsecase
	logger       *zap.Logger
	timezone     *time.Location
}

// NewChatUsecase creates the orchestrating chat usecase.
func NewChatUsecase(
	chatRepo domain.ChatRepository,
	llm domain.LLMClient,
	searchClient domain.SearchClient,
	reminderUC domain.ReminderUsecase,
	expenseUC domain.ExpenseUsecase,
	logger *zap.Logger,
	timezone *time.Location,
) domain.ChatUsecase {
	return &chatUsecase{
		chatRepo:     chatRepo,
		llm:          llm,
		searchClient: searchClient,
		reminderUC:   reminderUC,
		expenseUC:    expenseUC,
		logger:       logger,
		timezone:     timezone,
	}
}

// HandleMessage is the main entry point for all incoming messages.
// It classifies intent, routes to the appropriate handler, and returns the reply.
func (u *chatUsecase) HandleMessage(ctx context.Context, userMessage string) (string, error) {
	// 1. Save user message to history
	if err := u.saveMessage(ctx, domain.RoleUser, userMessage); err != nil {
		u.logger.Warn("failed to save user message", zap.Error(err))
	}

	// 2. Classify intent
	intent, err := u.llm.ClassifyIntent(ctx, userMessage)
	if err != nil {
		u.logger.Error("intent classification failed, falling back to general chat", zap.Error(err))
		intent = &domain.IntentResult{Intent: domain.IntentGeneralChat, Confidence: 0.5}
	}
	u.logger.Info("intent classified", zap.String("intent", intent.Intent), zap.Float64("confidence", intent.Confidence))

	// 3. Route to handler
	var reply string
	switch intent.Intent {
	case domain.IntentReminder:
		reply, err = u.handleReminder(ctx, userMessage)
	case domain.IntentExpense:
		reply, err = u.handleExpense(ctx, userMessage)
	case domain.IntentSearch:
		reply, err = u.handleSearch(ctx, userMessage)
	case domain.IntentReset:
		reply, err = u.handleReset(ctx)
	default:
		reply, err = u.handleGeneralChat(ctx, userMessage)
	}

	if err != nil {
		u.logger.Error("handler error", zap.String("intent", intent.Intent), zap.Error(err))
		reply = "Maaf, ada yang tidak beres saat memproses pesanmu. Coba lagi ya! 🙏"
	}

	// 4. Save assistant reply to history
	if err := u.saveMessage(ctx, domain.RoleAssistant, reply); err != nil {
		u.logger.Warn("failed to save assistant message", zap.Error(err))
	}

	return reply, nil
}

// handleReminder parses and processes reminder-related messages.
func (u *chatUsecase) handleReminder(ctx context.Context, message string) (string, error) {
	// Check if user is asking to list reminders
	listKeywords := []string{"daftar", "list", "apa aja", "aktif", "lihat"}
	for _, kw := range listKeywords {
		if strings.Contains(strings.ToLower(message), kw) {
			reminders, err := u.reminderUC.ListActive(ctx)
			if err != nil {
				return "", err
			}
			return FormatReminderList(reminders), nil
		}
	}

	// Check if user wants to cancel a reminder
	cancelKeywords := []string{"batalkan", "cancel", "hapus", "delete"}
	for _, kw := range cancelKeywords {
		if strings.Contains(strings.ToLower(message), kw) {
			return "Buat cancel reminder, sebutkan ID-nya wok. Ketik \"daftar reminder\" dulu untuk lihat ID reminder aktifmu. Biar gw atur pembatalannya😅", nil
		}
	}

	// Parse one or more reminders from natural language
	parsed, err := u.llm.ParseReminders(ctx, message)
	if err != nil {
		return "", fmt.Errorf("parse reminder: %w", err)
	}
	if len(parsed) == 0 {
		return "Gw gak paham maksud lu njir😹", nil
	}

	// Create all reminders and collect confirmations
	var lines []string
	for _, p := range parsed {
		scheduledAt, err := parseScheduledTime(p.ScheduledAt, u.timezone)
		if err != nil {
			u.logger.Warn("skip reminder: invalid scheduled_at", zap.String("raw", p.ScheduledAt), zap.Error(err))
			continue
		}

		reminder, err := u.reminderUC.CreateReminder(ctx, p.Title, scheduledAt, p.Recurrence)
		if err != nil {
			u.logger.Error("failed to create reminder", zap.String("title", p.Title), zap.Error(err))
			continue
		}

		recStr := "sekali"
		if reminder.Recurrence != nil && *reminder.Recurrence != "" {
			recStr = *reminder.Recurrence
		}
		lines = append(lines, fmt.Sprintf("📌 *%s*\n   ⏰ %s (%s)",
			reminder.Title,
			reminder.ScheduledAt.Format("Mon, 02 Jan 2006 15:04 WIB"),
			recStr,
		))
	}

	if len(lines) == 0 {
		return "Gagal membuat semua reminder. Coba lagi nanti cik! 🙏", nil
	}

	if len(lines) == 1 {
		return "✅ Reminder dibuat wok!\n\n" + lines[0], nil
	}

	return fmt.Sprintf("✅ %d reminder berhasil dibuat wok!\n\n%s", len(lines), strings.Join(lines, "\n\n")), nil
}

// handleExpense parses and records expense messages, or returns reports.
func (u *chatUsecase) handleExpense(ctx context.Context, message string) (string, error) {
	// Check if user wants a report
	reportKeywords := []string{"laporan", "rekap", "total", "berapa", "summary"}
	for _, kw := range reportKeywords {
		if strings.Contains(strings.ToLower(message), kw) {
			report, err := u.expenseUC.GetCurrentMonthReport(ctx)
			if err != nil {
				return "", err
			}
			return FormatExpenseReport(report), nil
		}
	}

	// Parse expense from natural language
	amount, category, description, err := u.llm.ParseExpense(ctx, message)
	if err != nil {
		return "", fmt.Errorf("parse expense: %w", err)
	}

	// Record expense
	expense, err := u.expenseUC.RecordExpense(ctx, amount, category, description, time.Now().In(u.timezone))
	if err != nil {
		return "", err
	}

	return FormatExpenseConfirmation(expense), nil
}

// handleSearch performs a web search and returns an LLM-synthesized answer.
func (u *chatUsecase) handleSearch(ctx context.Context, message string) (string, error) {
	results, err := u.searchClient.Search(ctx, message, maxSearchResults)
	if err != nil {
		u.logger.Error("search failed, falling back to LLM direct answer", zap.Error(err))
		return u.handleGeneralChat(ctx, message)
	}

	if len(results) == 0 {
		return u.handleGeneralChat(ctx, message)
	}

	// Build context from search results
	var contextParts []string
	for i, r := range results {
		contextParts = append(contextParts,
			fmt.Sprintf("[%d] %s\nURL: %s\n%s", i+1, r.Title, r.URL, r.Snippet),
		)
	}
	searchContext := strings.Join(contextParts, "\n\n---\n\n")

	// Build augmented message
	augmentedMessage := fmt.Sprintf(`Pertanyaan pengguna: %s

Berikut hasil pencarian web yang relevan:

%s

Berdasarkan hasil pencarian di atas, berikan jawaban yang informatif dan ringkas dalam Bahasa Indonesia. Sebutkan sumber jika relevan.`, message, searchContext)

	// Get history for context
	history, _ := u.chatRepo.GetRecentMessages(ctx, maxChatHistory)
	messages := make([]domain.ChatMessage, 0, len(history)+1)
	for _, h := range history {
		messages = append(messages, *h)
	}
	messages = append(messages, domain.ChatMessage{Role: domain.RoleUser, Message: augmentedMessage})

	return u.llm.Chat(ctx, messages)
}

// handleGeneralChat handles general conversation with context history.
func (u *chatUsecase) handleGeneralChat(ctx context.Context, message string) (string, error) {
	history, err := u.chatRepo.GetRecentMessages(ctx, maxChatHistory)
	if err != nil {
		u.logger.Warn("failed to get chat history", zap.Error(err))
	}

	messages := make([]domain.ChatMessage, 0, len(history)+1)
	for _, h := range history {
		messages = append(messages, *h)
	}
	// messages = append(messages, domain.ChatMessage{Role: domain.RoleUser, Message: message})

	return u.llm.Chat(ctx, messages)
}

// handleReset clears the chat history.
func (u *chatUsecase) handleReset(ctx context.Context) (string, error) {
	if err := u.chatRepo.ClearHistory(ctx); err != nil {
		return "", err
	}
	return "🔄 Obrolan kita direset! Mulai dari awal ya. Ada yang bisa aku bantu?", nil
}

// ResetContext clears the conversation history.
func (u *chatUsecase) ResetContext(ctx context.Context) error {
	return u.chatRepo.ClearHistory(ctx)
}

// GetHistory returns recent chat messages.
func (u *chatUsecase) GetHistory(ctx context.Context, limit int) ([]*domain.ChatMessage, error) {
	return u.chatRepo.GetRecentMessages(ctx, limit)
}

// saveMessage persists a chat message to the repository.
func (u *chatUsecase) saveMessage(ctx context.Context, role, message string) error {
	return u.chatRepo.SaveMessage(ctx, &domain.ChatMessage{
		Role:      role,
		Message:   message,
		CreatedAt: time.Now(),
	})
}

// parseScheduledTime tries multiple common datetime layouts to parse LLM output reliably.
func parseScheduledTime(val string, loc *time.Location) (time.Time, error) {
	val = strings.TrimSpace(val)
	if val == "" {
		return time.Time{}, fmt.Errorf("empty datetime string")
	}

	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z07:00",
		time.RFC3339,
		"2006-01-02 15:04:05 -0700",
		"2006-01-02",
	}

	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, val, loc); err == nil {
			// If only date was provided (00:00:00), set default to 08:00 AM
			if layout == "2006-01-02" {
				t = time.Date(t.Year(), t.Month(), t.Day(), 8, 0, 0, 0, loc)
			}
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("cannot parse %q as datetime", val)
}
