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

const receiptStateKey = "self" // single-user assistant: always same key

type chatUsecase struct {
	chatRepo     domain.ChatRepository
	llm          domain.LLMClient
	searchClient domain.SearchClient
	reminderUC   domain.ReminderUsecase
	expenseUC    domain.ExpenseUsecase
	logger       *zap.Logger
	timezone     *time.Location
	receiptStore *receiptStateStore // pending receipt awaiting confirmation
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
		receiptStore: newReceiptStateStore(),
	}
}

// HandleMessage is the main entry point for all incoming text messages.
// It classifies intent, routes to the appropriate handler, and returns the reply.
func (u *chatUsecase) HandleMessage(ctx context.Context, userMessage string) (string, error) {
	// 0. Check if there's a pending receipt waiting for confirmation/correction
	if pending, ok := u.receiptStore.get(receiptStateKey); ok {
		reply, handled := u.handleReceiptReply(ctx, userMessage, pending)
		if handled {
			if err := u.saveMessage(ctx, domain.RoleUser, userMessage); err != nil {
				u.logger.Warn("failed to save user message", zap.Error(err))
			}
			if err := u.saveMessage(ctx, domain.RoleAssistant, reply); err != nil {
				u.logger.Warn("failed to save assistant message", zap.Error(err))
			}
			return reply, nil
		}
	}

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

// HandleImageMessage processes an image sent via WhatsApp (e.g. a receipt photo).
func (u *chatUsecase) HandleImageMessage(ctx context.Context, imageData []byte, mimeType string, caption string) (string, error) {
	u.logger.Info("receipt image received", zap.String("mimeType", mimeType), zap.Int("bytes", len(imageData)))

	receipt, err := u.llm.ParseReceiptFromImage(ctx, imageData, mimeType)
	if err != nil {
		u.logger.Error("failed to parse receipt image", zap.Error(err))
		return "Waduh, gagal baca struknya nih wir 😅. Coba kirim ulang dengan foto yang lebih jelas ya.", nil
	}

	if receipt == nil || len(receipt.Items) == 0 {
		return "Hmm, kayaknya ini bukan struk deh wir, atau tulisannya kurang jelas. Coba foto ulang yang lebih terang ya 📸", nil
	}

	// Store pending receipt for confirmation
	u.receiptStore.set(receiptStateKey, receipt)

	u.logger.Info("receipt parsed, awaiting confirmation",
		zap.String("store", receipt.StoreName),
		zap.Int("items", len(receipt.Items)),
		zap.Float64("total", receipt.Total),
	)

	return FormatReceiptPreview(receipt), nil
}

// HandleAudioMessage processes an audio message sent via WhatsApp (e.g. a voice note).
func (u *chatUsecase) HandleAudioMessage(ctx context.Context, audioData []byte, mimeType string) (string, error) {
	u.logger.Info("audio message received", zap.String("mimeType", mimeType), zap.Int("bytes", len(audioData)))

	// Clasify audio intent
	intent, err := u.llm.ClassifyAudioIntent(ctx, audioData, mimeType)
	if err != nil {
		u.logger.Error("intent classification failed, falling back to general chat", zap.Error(err))
		intent = &domain.IntentResult{Intent: domain.IntentGeneralChat, Confidence: 0.5}
	}
	u.logger.Info("intent classified", zap.String("intent", intent.Intent), zap.Float64("confidence", intent.Confidence))

	// 3. Route to handler
	var reply string
	switch intent.Intent {
	case domain.IntentReminder:
		reply, err = u.handleReminder(ctx, intent.RawMessage)
	case domain.IntentExpense:
		reply, err = u.handleExpense(ctx, intent.RawMessage)
	case domain.IntentSearch:
		reply, err = u.handleSearch(ctx, intent.RawMessage)
	case domain.IntentReset:
		reply, err = u.handleReset(ctx)
	default:
		reply, err = u.handleGeneralChat(ctx, intent.RawMessage)
	}

	if err != nil {
		u.logger.Error("handler error", zap.String("intent", intent.Intent), zap.Error(err))
		reply = "Gw ga paham bahasa lu Jawa"
	}

	// Save user message to history
	if err := u.saveMessage(ctx, domain.RoleUser, intent.RawMessage); err != nil {
		u.logger.Warn("failed to save user message", zap.Error(err))
	}

	// Save assistant reply to history
	if err := u.saveMessage(ctx, domain.RoleAssistant, reply); err != nil {
		u.logger.Warn("failed to save assistant message", zap.Error(err))
	}

	return reply, nil
}

// handleReceiptReply handles user replies when a pending receipt is awaiting confirmation.
// Returns (reply, true) if the message was handled as a receipt command, (_, false) otherwise.
func (u *chatUsecase) handleReceiptReply(ctx context.Context, msg string, receipt *domain.ReceiptParsed) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(msg))

	// Cancel
	if lower == "batal" || lower == "cancel" || lower == "ga jadi" {
		u.receiptStore.clear(receiptStateKey)
		return "Oke, scan struk dibatalin wir. 👌", true
	}

	// Confirm & save all
	confirmKeywords := []string{"ya", "yes", "iya", "ok", "oke", "simpan", "save", "lanjut", "confirm", "benar", "bener", "josjis", "wis", "wes"}
	for _, kw := range confirmKeywords {
		if lower == kw {
			return u.saveReceiptItems(ctx, receipt)
		}
	}

	// Koreksi: "koreksi 1 nama=Nasi Goreng jumlah=25000 kategori=makan"
	if strings.HasPrefix(lower, "koreksi") {
		return u.handleReceiptCorrection(msg, receipt)
	}

	// Hapus item: "hapus 2"
	if strings.HasPrefix(lower, "hapus ") {
		return u.handleReceiptDelete(msg, receipt)
	}

	// Not a recognized receipt command — let normal flow handle it
	return "", false
}

// handleReceiptCorrection parses and applies user correction to a pending receipt item.
// Syntax: "koreksi <nomor> [nama=...] [jumlah=...] [kategori=...]"
func (u *chatUsecase) handleReceiptCorrection(msg string, receipt *domain.ReceiptParsed) (string, bool) {
	tokens := strings.Fields(msg)
	if len(tokens) < 2 {
		return "Format koreksi: _koreksi <nomor item> [nama=...] [jumlah=...] [kategori=...]_\nContoh: _koreksi 1 nama=Nasi Goreng jumlah=25000 kategori=makan_", true
	}

	var idx int
	if _, err := fmt.Sscanf(tokens[1], "%d", &idx); err != nil || idx < 1 || idx > len(receipt.Items) {
		return fmt.Sprintf("Nomor item tidak valid wir. Pilih antara 1-%d.", len(receipt.Items)), true
	}
	idx-- // 0-indexed

	for _, tok := range tokens[2:] {
		parts := strings.SplitN(tok, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])
		switch key {
		case "nama", "name", "deskripsi", "desc":
			receipt.Items[idx].Description = val
		case "jumlah", "harga", "amount", "price":
			var amount float64
			if _, err := fmt.Sscanf(val, "%f", &amount); err == nil {
				receipt.Items[idx].Amount = amount
			}
		case "kategori", "cat", "category":
			receipt.Items[idx].Category = val
		}
	}

	// Recalculate total
	total := 0.0
	for _, item := range receipt.Items {
		total += item.Amount
	}
	receipt.Total = total

	// Update the stored pending receipt
	u.receiptStore.set(receiptStateKey, receipt)

	return "✏️ Diupdate wir! Ini struk terbaru:\n\n" + FormatReceiptPreview(receipt), true
}

// handleReceiptDelete removes an item from the pending receipt.
// Syntax: "hapus <nomor>"
func (u *chatUsecase) handleReceiptDelete(msg string, receipt *domain.ReceiptParsed) (string, bool) {
	tokens := strings.Fields(msg)
	if len(tokens) < 2 {
		return "Format hapus: _hapus <nomor item>_\nContoh: _hapus 2_", true
	}

	var idx int
	if _, err := fmt.Sscanf(tokens[1], "%d", &idx); err != nil || idx < 1 || idx > len(receipt.Items) {
		return fmt.Sprintf("Nomor item tidak valid wir. Pilih antara 1-%d.", len(receipt.Items)), true
	}
	idx-- // 0-indexed

	removed := receipt.Items[idx]
	receipt.Items = append(receipt.Items[:idx], receipt.Items[idx+1:]...)

	// Recalculate total
	total := 0.0
	for _, item := range receipt.Items {
		total += item.Amount
	}
	receipt.Total = total
	u.receiptStore.set(receiptStateKey, receipt)

	if len(receipt.Items) == 0 {
		u.receiptStore.clear(receiptStateKey)
		return fmt.Sprintf("🗑️ Item *%s* dihapus. Struk kosong wir, scan dibatalin deh.", removed.Description), true
	}

	return fmt.Sprintf("🗑️ Item *%s* dihapus wir!\n\nSisa struk:\n", removed.Description) + FormatReceiptPreview(receipt), true
}

// saveReceiptItems records all pending receipt items as expenses.
func (u *chatUsecase) saveReceiptItems(ctx context.Context, receipt *domain.ReceiptParsed) (string, bool) {
	var saved []domain.ReceiptItem
	now := time.Now().In(u.timezone)

	for _, item := range receipt.Items {
		_, err := u.expenseUC.RecordExpense(ctx, item.Amount, item.Category, item.Description, now)
		if err != nil {
			u.logger.Error("failed to save receipt item", zap.String("item", item.Description), zap.Error(err))
			continue
		}
		saved = append(saved, item)
	}

	u.receiptStore.clear(receiptStateKey)

	if len(saved) == 0 {
		return "Gagal menyimpan semua item wir 😭. Coba lagi ya!", true
	}

	return FormatReceiptSaved(saved, receipt.StoreName), true
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
// It skips saving if message is empty to avoid sending blank parts to the LLM.
func (u *chatUsecase) saveMessage(ctx context.Context, role, message string) error {
	if strings.TrimSpace(message) == "" {
		return nil
	}
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
