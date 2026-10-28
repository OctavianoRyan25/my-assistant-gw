package usecase

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
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
	memoryRepo   domain.MemoryRepository
	llm          domain.LLMClient
	searchClient domain.SearchClient
	reminderUC   domain.ReminderUsecase
	expenseUC    domain.ExpenseUsecase
	nutritionUC  domain.NutritionUsecase // untuk perhitungan kalori dari gambar makanan
	workoutUC    domain.WorkoutUsecase
	budgetUC     domain.BudgetUsecase
	logger       *zap.Logger
	timezone     *time.Location
	receiptStore *receiptStateStore // pending receipt awaiting confirmation
}

// NewChatUsecase creates the orchestrating chat usecase.
func NewChatUsecase(
	chatRepo domain.ChatRepository,
	memoryRepo domain.MemoryRepository,
	llm domain.LLMClient,
	searchClient domain.SearchClient,
	reminderUC domain.ReminderUsecase,
	expenseUC domain.ExpenseUsecase,
	nutritionUC domain.NutritionUsecase,
	workoutUC domain.WorkoutUsecase,
	budgetUC domain.BudgetUsecase,
	logger *zap.Logger,
	timezone *time.Location,
) domain.ChatUsecase {
	return &chatUsecase{
		chatRepo:     chatRepo,
		memoryRepo:   memoryRepo,
		llm:          llm,
		searchClient: searchClient,
		reminderUC:   reminderUC,
		expenseUC:    expenseUC,
		nutritionUC:  nutritionUC,
		workoutUC:    workoutUC,
		budgetUC:     budgetUC,
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

	// 1b. Analisis memori kepribadian & fakta user secara background (asynchronous, tanpa blocking respon chat)
	go u.analyzeMemoriesAsync(userMessage)

	// 2. Classify intent — sertakan 3 pesan terakhir sebagai konteks supaya AI paham referensi ambigu
	classifyInput := userMessage
	if recentHistory, err := u.chatRepo.GetRecentMessages(ctx, 3); err == nil && len(recentHistory) > 1 {
		var sb strings.Builder
		sb.WriteString("KONTEKS PERCAKAPAN SEBELUMNYA (hanya untuk referensi):\n")
		for _, h := range recentHistory[:len(recentHistory)-1] {
			role := "User"
			if h.Role == domain.RoleAssistant {
				role = "Assistant"
			}
			sb.WriteString(fmt.Sprintf("%s: %s\n", role, h.Message))
		}
		sb.WriteString("\nPESAN TERBARU: ")
		sb.WriteString(userMessage)
		classifyInput = sb.String()
	}

	intent, err := u.llm.ClassifyIntent(ctx, classifyInput)
	if err != nil {
		u.logger.Error("intent classification failed, falling back to general chat", zap.Error(err))
		intent = &domain.IntentResult{Intent: domain.IntentGeneralChat, Confidence: 0.5}
	}
	u.logger.Info("intent classified",
		zap.String("intent", intent.Intent),
		zap.Float64("confidence", intent.Confidence),
		zap.String("message", userMessage),
	)

	// 2b. Jika confidence rendah, jangan paksa ke handler spesifik.
	// Threshold 0.75: hanya route ke handler khusus kalau LLM cukup yakin.
	if intent.Confidence < 0.75 && intent.Intent != domain.IntentGeneralChat {
		u.logger.Info("low confidence — fallback to general_chat",
			zap.String("original_intent", intent.Intent),
			zap.Float64("confidence", intent.Confidence),
		)
		intent.Intent = domain.IntentGeneralChat
	}

	// 3. Route to handler
	var reply string
	switch intent.Intent {
	case domain.IntentReminder:
		reply, err = u.handleReminder(ctx, userMessage)
	case domain.IntentExpense:
		reply, err = u.handleExpense(ctx, userMessage)
	case domain.IntentWorkout:
		reply, err = u.handleWorkout(ctx, userMessage)
	case domain.IntentBudget:
		reply, err = u.handleBudget(ctx, userMessage)
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

// HandleImageMessage processes an image sent via WhatsApp.
// It first classifies the image intent (calorie/receipt/workout/general_chat),
// then routes to the appropriate handler.
func (u *chatUsecase) HandleImageMessage(ctx context.Context, imageData []byte, mimeType string, caption string) (string, error) {
	u.logger.Info("image received", zap.String("mimeType", mimeType), zap.Int("bytes", len(imageData)))

	// Classify image intent using Gemini Vision
	intent, err := u.llm.ClassifyImageIntent(ctx, imageData, mimeType)
	if err != nil {
		u.logger.Warn("image intent classification failed", zap.Error(err))
		return "Waduh, gagal analisa gambarnya wir 😅. Coba kirim ulang dengan foto yang lebih jelas ya.", nil
	}
	u.logger.Info("image intent classified", zap.String("intent", intent.Intent), zap.Float64("confidence", intent.Confidence))

	switch intent.Intent {
	case domain.IntentCalorie:
		return u.handleCalorieImage(ctx, imageData, mimeType, caption)
	case domain.IntentWorkout:
		return u.handleWorkoutImage(ctx, imageData, mimeType, caption)
	case domain.IntentReceipt:
		return u.handleReceiptImage(ctx, imageData, mimeType)
	default:
		// For general_chat or unknown, if there's a caption treat it as a message
		if strings.TrimSpace(caption) != "" {
			return u.handleGeneralChat(ctx, caption)
		}
		return "Gambar diterima wir, tapi gw belum bisa bantu banyak untuk jenis gambar ini 🤔. Kalau mau hitung kalori kirim foto makanan, atau struk belanja kalau mau catat pengeluaran.", nil
	}
}

// handleCalorieImage processes a food image and logs the estimated calories.
func (u *chatUsecase) handleCalorieImage(ctx context.Context, imageData []byte, mimeType, notes string) (string, error) {
	if u.nutritionUC == nil {
		return "Fitur hitung kalori belum dikonfigurasi wir 😅", nil
	}

	food, summary, err := u.nutritionUC.LogMealFromImage(ctx, imageData, mimeType, notes)
	if err != nil {
		u.logger.Error("failed to log meal from image", zap.Error(err))
		return "Waduh, gagal analisa makanannya wir 😅. Pastikan foto makanan cukup jelas ya, atau coba kirim ulang.", nil
	}

	return FormatMealConfirmation(food, summary), nil
}

// handleReceiptImage processes a receipt/invoice image and records expense items.
func (u *chatUsecase) handleReceiptImage(ctx context.Context, imageData []byte, mimeType string) (string, error) {
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

// handleWorkoutImageFallback handles workout images — currently redirects user to text input
// since WorkoutUsecase is not wired through chatUsecase.
func (u *chatUsecase) handleWorkoutImage(ctx context.Context, imageData []byte, mimeType string, caption string) (string, error) {
	if u.workoutUC == nil {
		return "Fitur workoit belum dikonfigurasi wir 😅", nil
	}

	workout, err := u.workoutUC.RecordWorkoutFromImage(ctx, imageData, mimeType, caption)
	if err != nil {
		u.logger.Error("failed to log workout from image", zap.Error(err))
		return "Waduh, gagal analisa workoutnya wir 😅. Pastikan foto workout cukup jelas ya, atau coba kirim ulang.", nil
	}

	return FormatWorkoutConfirmation(workout), nil
}

// handleWorkout processes workout-related text messages: recording a new workout, listing workouts, deleting workouts, or fetching stats.
func (u *chatUsecase) handleWorkout(ctx context.Context, message string) (string, error) {
	if u.workoutUC == nil {
		return "Fitur workout belum dikonfigurasi wir 😅", nil
	}

	cleanMsg := strings.TrimSpace(message)
	lower := strings.ToLower(cleanMsg)

	// 1. Cek apakah user ingin menghapus / membatalkan catatan workout.
	// Butuh context "workout" atau ID angka di pesan agar tidak false-positive.
	deleteVerbPresent := false
	for _, kw := range []string{"hapus", "delete"} {
		if strings.Contains(lower, kw) {
			deleteVerbPresent = true
			break
		}
	}
	workoutContextPresent := strings.Contains(lower, "workout") ||
		strings.Contains(lower, "olahraga") ||
		strings.Contains(lower, "latihan") ||
		strings.Contains(lower, "gym") ||
		strings.Contains(lower, "lari") ||
		strings.Contains(lower, "jogging")
	hasIDInMessage := regexp.MustCompile(`\b\d+\b`).MatchString(lower)
	isDelete := deleteVerbPresent && (workoutContextPresent || hasIDInMessage || strings.Contains(lower, "terakhir") || strings.Contains(lower, "last"))

	if isDelete {
		// A. Hapus workout terakhir / tadi
		if strings.Contains(lower, "terakhir") || strings.Contains(lower, "tadi") || strings.Contains(lower, "last") || strings.Contains(lower, "latest") {
			deleted, err := u.workoutUC.DeleteLatestWorkout(ctx)
			if err != nil {
				return "Belum ada riwayat workout yang bisa dihapus wir, atau catatan tidak ditemukan.", nil
			}
			return fmt.Sprintf("🗑️ *Workout terakhir berhasil dihapus wir!*\n\n• ID: *%d*\n• Aktivitas: *%s*\n• Tipe: %s", deleted.ID, deleted.Title, deleted.Type), nil
		}

		// B. Hapus berdasarkan ID angka (contoh: "hapus workout 5", "delete workout 12", "hapus 5")
		numRegex := regexp.MustCompile(`\b(\d+)\b`)
		matches := numRegex.FindStringSubmatch(lower)
		if len(matches) > 1 {
			if id, err := strconv.ParseInt(matches[1], 10, 64); err == nil && id > 0 {
				deleted, err := u.workoutUC.DeleteWorkout(ctx, id)
				if err != nil {
					return fmt.Sprintf("Workout dengan ID %d tidak ditemukan wir 😅. Ketik *list workout* untuk mengecek ID yang valid.", id), nil
				}
				return fmt.Sprintf("🗑️ *Workout berhasil dihapus wir!*\n\n• ID: *%d*\n• Aktivitas: *%s*\n• Tipe: %s", deleted.ID, deleted.Title, deleted.Type), nil
			}
		}

		// C. Jika user hanya mengetik "hapus workout" tanpa ID / spesifikasi
		return "Buat menghapus catatan workout, sebutkan ID-nya (contoh: *hapus workout 5*) atau ketik *hapus workout terakhir*.\n\nKetik *list workout* untuk melihat daftar riwayat dan ID workoutmu ya wir! 🏋️", nil
	}

	// 2. Cek apakah user ingin melihat daftar/riwayat workout
	// Gunakan frasa spesifik agar tidak false-positive pada kata umum "lihat"
	workoutListPhrases := []string{
		"list workout", "daftar workout", "riwayat workout", "history workout",
		"lihat workout", "show workout", "workout apa aja", "workout apa saja",
		"catatan workout", "log workout",
	}
	for _, kw := range workoutListPhrases {
		if strings.Contains(lower, kw) {
			workouts, err := u.workoutUC.GetRecentWorkouts(ctx, 10)
			if err != nil {
				return "", fmt.Errorf("get recent workouts: %w", err)
			}
			return FormatWorkoutList(workouts), nil
		}
	}
	// Fallback kata tunggal yang sudah cukup eksplisit
	for _, kw := range []string{"list", "daftar", "riwayat", "history"} {
		if lower == kw || lower == kw+" workout" || lower == kw+" olahraga" || lower == kw+" latihan" {
			workouts, err := u.workoutUC.GetRecentWorkouts(ctx, 10)
			if err != nil {
				return "", fmt.Errorf("get recent workouts: %w", err)
			}
			return FormatWorkoutList(workouts), nil
		}
	}

	// 3. Cek apakah user ingin melihat statistik/rekap workout bulanan
	// Gunakan kata kunci yang spesifik workout agar tidak bentrok dengan expense report
	workoutStatContext := strings.Contains(lower, "statistik") ||
		strings.Contains(lower, "stats") ||
		strings.Contains(lower, "summary workout") ||
		strings.Contains(lower, "rekap workout") ||
		strings.Contains(lower, "laporan workout") ||
		strings.Contains(lower, "progress workout") ||
		strings.Contains(lower, "bulan ini") && (strings.Contains(lower, "workout") || strings.Contains(lower, "olahraga") || strings.Contains(lower, "latihan"))
	if workoutStatContext {
		stats, err := u.workoutUC.GetCurrentMonthStats(ctx)
		if err != nil {
			return "", fmt.Errorf("get workout stats: %w", err)
		}
		return FormatWorkoutReport(stats), nil
	}

	// 4. Jika bukan perintah list/delete/stats, catat sebagai workout baru
	workout, err := u.workoutUC.RecordWorkoutFromText(ctx, message)
	if err != nil {
		u.logger.Warn("message not parsed as workout", zap.String("message", message), zap.Error(err))
		return "Waduh, pesanmu tidak terdeteksi sebagai aktivitas workout fisik wir 😅.\nContoh pencatatan: *\"Gym push day 60 menit\"* atau *\"Jogging 5km 40 menit\"*.\n\nUntuk melihat riwayat atau menghapus, ketik *list workout* atau *hapus workout terakhir*.", nil
	}

	return FormatWorkoutConfirmation(workout), nil
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
	case domain.IntentWorkout:
		reply, err = u.handleWorkout(ctx, intent.RawMessage)
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

	if strings.TrimSpace(intent.RawMessage) != "" {
		go u.analyzeMemoriesAsync(intent.RawMessage)
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
	cleanMsg := strings.TrimSpace(message)
	lower := strings.ToLower(cleanMsg)

	// 1. Cek apakah user ingin membatalkan atau menghapus reminder.
	// Supaya tidak false-positive, pesan harus mengandung domain context ("reminder" atau "jadwal")
	// ATAU kata hapus/batalkan yang dikombinasi dengan ID angka.
	deleteVerbPresent := false
	for _, kw := range []string{"batalkan", "hapus", "delete", "cancel"} {
		if strings.Contains(lower, kw) {
			deleteVerbPresent = true
			break
		}
	}
	reminderContextPresent := strings.Contains(lower, "reminder") ||
		strings.Contains(lower, "jadwal") ||
		strings.Contains(lower, "alarm") ||
		strings.Contains(lower, "ingatkan")
	hasIDInMessage := regexp.MustCompile(`\b\d+\b`).MatchString(lower)
	isDelete := deleteVerbPresent && (reminderContextPresent || hasIDInMessage || strings.Contains(lower, "terakhir") || strings.Contains(lower, "last"))

	if isDelete {
		// A. Hapus reminder terakhir / tadi
		if strings.Contains(lower, "terakhir") || strings.Contains(lower, "tadi") || strings.Contains(lower, "last") || strings.Contains(lower, "latest") {
			deleted, err := u.reminderUC.DeleteLatest(ctx)
			if err != nil {
				return "Belum ada reminder yang bisa dihapus wir, atau catatan tidak ditemukan.", nil
			}
			recStr := "sekali"
			if deleted.Recurrence != nil && *deleted.Recurrence != "" {
				recStr = *deleted.Recurrence
			}
			return fmt.Sprintf("🗑️ *Reminder terakhir berhasil dihapus dari database wir!*\n\n• ID: *%d*\n• Judul: *%s*\n• Jadwal: %s (%s)",
				deleted.ID,
				deleted.Title,
				deleted.ScheduledAt.In(u.timezone).Format("Mon, 02 Jan 2006 15:04 WIB"),
				recStr,
			), nil
		}

		// B. Hapus berdasarkan ID angka (contoh: "hapus reminder 5", "batalkan reminder 2", "delete id 10")
		numRegex := regexp.MustCompile(`\b(\d+)\b`)
		matches := numRegex.FindStringSubmatch(lower)
		if len(matches) > 1 {
			if id, err := strconv.ParseInt(matches[1], 10, 64); err == nil && id > 0 {
				deleted, err := u.reminderUC.Delete(ctx, id)
				if err != nil {
					return fmt.Sprintf("Reminder dengan ID %d tidak ditemukan di database wir 😅. Ketik *daftar reminder* untuk mengecek ID aktifmu.", id), nil
				}
				recStr := "sekali"
				if deleted.Recurrence != nil && *deleted.Recurrence != "" {
					recStr = *deleted.Recurrence
				}
				return fmt.Sprintf("🗑️ *Reminder berhasil dihapus dari database wir!*\n\n• ID: *%d*\n• Judul: *%s*\n• Jadwal: %s (%s)",
					deleted.ID,
					deleted.Title,
					deleted.ScheduledAt.In(u.timezone).Format("Mon, 02 Jan 2006 15:04 WIB"),
					recStr,
				), nil
			}
		}

		// C. Cek apakah user menyebutkan kata kunci judul reminder aktif
		activeReminders, _ := u.reminderUC.ListActive(ctx)
		if len(activeReminders) > 0 {
			searchQuery := lower
			for _, kw := range []string{"batalkan", "batal", "cancel", "hapus", "delete", "reminder", "jadwal", "yang", "tolong", "dong", "wir"} {
				searchQuery = strings.ReplaceAll(searchQuery, kw, "")
			}
			searchQuery = strings.TrimSpace(searchQuery)

			if len(searchQuery) >= 3 {
				var matched []*domain.Reminder
				for _, r := range activeReminders {
					if strings.Contains(strings.ToLower(r.Title), searchQuery) {
						matched = append(matched, r)
					}
				}
				if len(matched) == 1 {
					deleted, err := u.reminderUC.Delete(ctx, matched[0].ID)
					if err == nil {
						recStr := "sekali"
						if deleted.Recurrence != nil && *deleted.Recurrence != "" {
							recStr = *deleted.Recurrence
						}
						return fmt.Sprintf("🗑️ *Reminder berhasil dihapus dari database wir!*\n\n• ID: *%d*\n• Judul: *%s*\n• Jadwal: %s (%s)",
							deleted.ID,
							deleted.Title,
							deleted.ScheduledAt.In(u.timezone).Format("Mon, 02 Jan 2006 15:04 WIB"),
							recStr,
						), nil
					}
				}
			}

			return "Untuk menghapus reminder, sebutkan ID-nya (contoh: *hapus reminder 5*) atau ketik *hapus reminder terakhir*.\n\n" + FormatReminderList(activeReminders), nil
		}

		return "Saat ini tidak ada reminder aktif yang bisa dihapus wir.", nil
	}

	// 2. Check if user is asking to list reminders
	// Pakai frasa spesifik agar tidak false-positive terhadap kata umum seperti "lihat"
	reminderListKeywords := []string{
		"daftar reminder", "list reminder", "lihat reminder", "show reminder",
		"reminder aktif", "reminder apa aja", "reminder apa saja", "jadwal aktif",
		"daftar jadwal", "list jadwal", "alarm aktif",
	}
	for _, kw := range reminderListKeywords {
		if strings.Contains(lower, kw) {
			reminders, err := u.reminderUC.ListActive(ctx)
			if err != nil {
				return "", err
			}
			return FormatReminderList(reminders), nil
		}
	}
	// Fallback: kata tunggal yang sudah cukup spesifik dalam konteks reminder
	for _, kw := range []string{"daftar", "list", "aktif"} {
		if lower == kw || lower == kw+" reminder" || lower == kw+" jadwal" {
			reminders, err := u.reminderUC.ListActive(ctx)
			if err != nil {
				return "", err
			}
			return FormatReminderList(reminders), nil
		}
	}

	// 3. Parse one or more reminders from natural language
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
		lines = append(lines, fmt.Sprintf("📌 [ID: %d] *%s*\n   ⏰ %s (%s)",
			reminder.ID,
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

// handleExpense parses and records expense messages, or returns reports/lists.
func (u *chatUsecase) handleExpense(ctx context.Context, message string) (string, error) {
	lowerMsg := strings.ToLower(message)

	// ── 1. Deteksi intent HAPUS pengeluaran ──────────────────────────────────
	// Harus ada kata hapus/delete DAN konteks pengeluaran/expense.
	deleteVerbPresent := strings.Contains(lowerMsg, "hapus") || strings.Contains(lowerMsg, "delete") || strings.Contains(lowerMsg, "batalkan")
	expenseContextPresent := strings.Contains(lowerMsg, "pengeluaran") || strings.Contains(lowerMsg, "expense") ||
		strings.Contains(lowerMsg, "transaksi") || strings.Contains(lowerMsg, "catat")
	hasIDInMsg := regexp.MustCompile(`\b\d+\b`).MatchString(lowerMsg)
	if deleteVerbPresent && (expenseContextPresent || hasIDInMsg) {
		// Hapus berdasarkan ID jika ada angka
		numRegex := regexp.MustCompile(`\b(\d+)\b`)
		matches := numRegex.FindStringSubmatch(lowerMsg)
		if len(matches) > 1 {
			if id, parseErr := strconv.ParseInt(matches[1], 10, 64); parseErr == nil && id > 0 {
				if err := u.expenseUC.DeleteExpense(ctx, id); err != nil {
					return fmt.Sprintf("Pengeluaran dengan ID %d tidak ditemukan wir 😅. Ketik *list pengeluaran* untuk melihat daftar.", id), nil
				}
				return fmt.Sprintf("🗑️ *Pengeluaran ID %d berhasil dihapus wir!*", id), nil
			}
		}
		return "Untuk menghapus pengeluaran, sebutkan ID-nya (contoh: *hapus pengeluaran 5*).\nKetik *list pengeluaran* untuk melihat daftar ID dulu ya wir! 💸", nil
	}

	// ── 2. Deteksi intent LIST / DETAIL / LIHAT daftar pengeluaran ──────────────────
	listKeywords := []string{
		"list pengeluaran", "daftar pengeluaran", "lihat pengeluaran", "show pengeluaran",
		"riwayat pengeluaran", "history pengeluaran", "transaksi terakhir",
		"pengeluaran apa aja", "pengeluaran apa saja", "catatan pengeluaran",
		"list expense", "daftar expense", "detail pengeluaran", "rincian pengeluaran",
		"pengeluaran hari ini", "pengeluaran today", "expense hari ini", "biaya hari ini",
	}

	now := time.Now().In(u.timezone)
	targetYear, targetMonth, periodStr := ParseExpenseTargetPeriod(lowerMsg, now)
	detectedCat := detectExpenseCategory(lowerMsg)
	isPastPeriod := targetYear != now.Year() || targetMonth != int(now.Month())
	isToday := strings.Contains(lowerMsg, "hari ini") || strings.Contains(lowerMsg, "today")

	wantsList := false
	for _, kw := range listKeywords {
		if strings.Contains(lowerMsg, kw) {
			wantsList = true
			break
		}
	}
	if !wantsList && (expenseContextPresent || strings.Contains(lowerMsg, "biaya")) {
		wantsList = strings.Contains(lowerMsg, "detail") || strings.Contains(lowerMsg, "rincian")
	}

	if !wantsList && isToday && (expenseContextPresent || strings.Contains(lowerMsg, "pengeluaran") || strings.Contains(lowerMsg, "expense") || strings.Contains(lowerMsg, "biaya") || detectedCat != "") {
		isRecordAction := strings.Contains(lowerMsg, "catat") || strings.Contains(lowerMsg, "tambah") || strings.Contains(lowerMsg, "input") || strings.Contains(lowerMsg, "masukkan") || strings.Contains(lowerMsg, "tambahkan")
		isReportAction := strings.Contains(lowerMsg, "laporan") || strings.Contains(lowerMsg, "rekap") || strings.Contains(lowerMsg, "summary") || strings.Contains(lowerMsg, "report") || strings.Contains(lowerMsg, "berapa")
		if !isRecordAction && !isReportAction {
			wantsList = true
		}
	}

	if wantsList {
		if isToday {
			if detectedCat != "" {
				expenses, err := u.expenseUC.GetTodayExpensesByCategory(ctx, detectedCat)
				if err != nil {
					return "", err
				}
				return FormatDetailExpenseList(expenses, periodStr, detectedCat), nil
			}
			expenses, err := u.expenseUC.GetTodayExpenses(ctx)
			if err != nil {
				return "", err
			}
			return FormatDetailExpenseList(expenses, periodStr, ""), nil
		}
		if detectedCat != "" {
			expenses, err := u.expenseUC.GetDetailMonthReportByCategory(ctx, targetYear, targetMonth, detectedCat)
			if err != nil {
				return "", err
			}
			return FormatDetailExpenseList(expenses, periodStr, detectedCat), nil
		}
		expenses, err := u.expenseUC.GetDetailMonthlyReport(ctx, targetYear, targetMonth)
		if err != nil {
			return "", err
		}
		return FormatDetailExpenseList(expenses, periodStr, ""), nil
	}

	// ── 3. Deteksi intent LAPORAN / REKAP / SUMMARY DAN APAKAH USER MENYURUH MENGEMBALIKAN DETAIL NYA ─────────────────────────
	expenseReportContext := strings.Contains(lowerMsg, "laporan") ||
		strings.Contains(lowerMsg, "rekap") ||
		strings.Contains(lowerMsg, "summary") ||
		strings.Contains(lowerMsg, "report") ||
		strings.Contains(lowerMsg, "pengeluaran bulan") ||
		strings.Contains(lowerMsg, "total pengeluaran") ||
		strings.Contains(lowerMsg, "habis berapa") ||
		(strings.Contains(lowerMsg, "berapa") && (strings.Contains(lowerMsg, "pengeluaran") || strings.Contains(lowerMsg, "habis") || strings.Contains(lowerMsg, "keluar") || strings.Contains(lowerMsg, "belanja"))) ||
		((expenseContextPresent || strings.Contains(lowerMsg, "pengeluaran")) && isPastPeriod)
	if expenseReportContext {
		if isToday {
			if strings.Contains(lowerMsg, "detail") || strings.Contains(lowerMsg, "rincian") || detectedCat != "" {
				if detectedCat != "" {
					expenses, err := u.expenseUC.GetTodayExpensesByCategory(ctx, detectedCat)
					if err != nil {
						return "", err
					}
					return FormatDetailExpenseList(expenses, periodStr, detectedCat), nil
				}
				expenses, err := u.expenseUC.GetTodayExpenses(ctx)
				if err != nil {
					return "", err
				}
				return FormatDetailExpenseList(expenses, periodStr, ""), nil
			}

			report, err := u.expenseUC.GetTodayExpenseReport(ctx)
			if err != nil {
				return "", err
			}
			return FormatExpenseReport(report), nil
		}
		// Jika meminta kategori spesifik atau ada kata kunci detail/rincian
		if detectedCat != "" {
			expenses, err := u.expenseUC.GetDetailMonthReportByCategory(ctx, targetYear, targetMonth, detectedCat)
			if err != nil {
				return "", err
			}
			return FormatDetailExpenseList(expenses, periodStr, detectedCat), nil
		}
		if strings.Contains(lowerMsg, "detail") || strings.Contains(lowerMsg, "rincian") {
			expenses, err := u.expenseUC.GetDetailMonthlyReport(ctx, targetYear, targetMonth)
			if err != nil {
				return "", err
			}
			return FormatDetailExpenseList(expenses, periodStr, ""), nil
		}

		report, err := u.expenseUC.GetMonthlyReport(ctx, targetYear, targetMonth)
		if err != nil {
			return "", err
		}
		return FormatExpenseReport(report), nil
	}

	// ── 4. Parse & catat pengeluaran baru dari bahasa natural ───────────────
	amount, category, description, err := u.llm.ParseExpense(ctx, message)
	if err != nil {
		return "", fmt.Errorf("parse expense: %w", err)
	}

	// Guard: jangan catat kalau amount = 0 (kemungkinan user minta list/tanya, bukan mencatat)
	if amount <= 0 {
		return "Hmm, gw ga nangkep nominal pengeluarannya wir 🤔\n" +
			"Kalau mau *catat*: contoh _\"keluar 50rb buat makan\"_\n" +
			"Kalau mau *lihat daftar*: ketik _\"list pengeluaran\"_\n" +
			"Kalau mau *rekap bulanan*: ketik _\"laporan pengeluaran\"_", nil
	}

	// Record expense
	expense, err := u.expenseUC.RecordExpense(ctx, amount, category, description, time.Now().In(u.timezone))
	if err != nil {
		return "", err
	}

	reply := FormatExpenseConfirmation(expense)
	if u.budgetUC != nil {
		if bStatus, alertTriggered, _ := u.budgetUC.CheckExpenseThreshold(ctx, amount); alertTriggered && bStatus != nil && bStatus.WarningMessage != "" {
			reply += "\n\n" + bStatus.WarningMessage
		}
	}

	return reply, nil
}

// handleBudget handles setting, viewing, and checking monthly budget / salary thresholds.
func (u *chatUsecase) handleBudget(ctx context.Context, message string) (string, error) {
	if u.budgetUC == nil {
		return "Fitur budget belum diaktifkan wir.", nil
	}

	lowerMsg := strings.ToLower(strings.TrimSpace(message))
	now := time.Now().In(u.timezone)
	targetYear, targetMonth, _ := ParseExpenseTargetPeriod(lowerMsg, now)

	// 1. Cek apakah user berniat MENYETEL / MENGUBAH budget atau gaji
	salary, threshold, hasSalary, hasThreshold := ParseBudgetSettingInput(lowerMsg)

	isSettingVerb := strings.Contains(lowerMsg, "set") ||
		strings.Contains(lowerMsg, "atur") ||
		strings.Contains(lowerMsg, "ubah") ||
		strings.Contains(lowerMsg, "ganti") ||
		strings.Contains(lowerMsg, "masukkan") ||
		strings.Contains(lowerMsg, "input") ||
		strings.Contains(lowerMsg, "pasang") ||
		strings.Contains(lowerMsg, "kasih budget") ||
		strings.Contains(lowerMsg, "bagiin budget")

	if hasSalary || (isSettingVerb && hasThreshold) {
		if !hasSalary {
			// Hanya mengubah ambang batas peringatan (threshold)
			currStatus, err := u.budgetUC.GetBudgetStatus(ctx, targetYear, targetMonth)
			if err == nil && currStatus != nil && currStatus.MonthlySalary > 0 {
				budget, setErr := u.budgetUC.SetBudget(ctx, currStatus.MonthlySalary, threshold)
				if setErr != nil {
					return "", setErr
				}
				status, _ := u.budgetUC.GetBudgetStatus(ctx, targetYear, targetMonth)
				return fmt.Sprintf("✅ *Ambang batas peringatan budget berhasil diubah jadi %.0f%%!*\n\n%s",
					budget.WarningThresholdPercent,
					FormatBudgetStatus(status),
				), nil
			}
			return "Berapa nominal budget/gaji bulanan yang mau disetel wir? 🤔\nContoh: *set budget 5jt* atau *gaji 10jt threshold 80%*", nil
		}

		if !hasThreshold {
			threshold = domain.DefaultWarningThresholdPercent
		}

		budget, err := u.budgetUC.SetBudget(ctx, salary, threshold)
		if err != nil {
			return "", err
		}

		status, err := u.budgetUC.GetBudgetStatus(ctx, targetYear, targetMonth)
		if err != nil {
			return "", err
		}

		return fmt.Sprintf("✅ *Budget bulanan berhasil disetel: Rp %s (Threshold: %.0f%%)!*\n\n%s",
			formatRupiah(budget.MonthlySalary),
			budget.WarningThresholdPercent,
			FormatBudgetStatus(status),
		), nil
	}

	// 2. Default: Tampilkan status budget bulan ini (atau bulan yang diminta)
	// Contoh: "budget bulan ini", "cek budget", "sisa budget", "uangku tinggal berapa", "sisa uang bulan ini"
	status, err := u.budgetUC.GetBudgetStatus(ctx, targetYear, targetMonth)
	if err != nil {
		return "", err
	}

	return FormatBudgetStatus(status), nil
}

// ParseBudgetSettingInput parses salary amount and warning threshold percentage from text.
func ParseBudgetSettingInput(msg string) (salary float64, threshold float64, hasSalary bool, hasThreshold bool) {
	lower := strings.ToLower(msg)

	// 1. Parse threshold jika ada (contoh: "threshold 85%", "ambang batas 75", "warning 80%")
	thresholdRegexes := []*regexp.Regexp{
		regexp.MustCompile(`(?:threshold|warning|peringatan|ambang|batas)\s*(?:peringatan\s*)?(\d{1,3})\s*%?`),
		regexp.MustCompile(`(\d{1,3})\s*%\s*(?:threshold|warning|peringatan|ambang|batas)?`),
	}
	for _, re := range thresholdRegexes {
		if m := re.FindStringSubmatch(lower); len(m) > 1 {
			if val, err := strconv.ParseFloat(m[1], 64); err == nil && val > 0 && val <= 100 {
				threshold = val
				hasThreshold = true
				break
			}
		}
	}

	// 2. Parse nominal gaji / budget
	// A. Juta: "10jt", "10 jt", "5.5 juta", "5,5jt"
	jutaRegex := regexp.MustCompile(`(\d+(?:[.,]\d+)?)\s*(?:jt|juta|million)\b`)
	if m := jutaRegex.FindStringSubmatch(lower); len(m) > 1 {
		numStr := strings.ReplaceAll(m[1], ",", ".")
		if val, err := strconv.ParseFloat(numStr, 64); err == nil && val > 0 {
			salary = val * 1000000
			hasSalary = true
			return
		}
	}

	// B. Ribu: "500rb", "500 ribu", "500k"
	ribuRegex := regexp.MustCompile(`(\d+(?:[.,]\d+)?)\s*(?:rb|ribu|k)\b`)
	if m := ribuRegex.FindStringSubmatch(lower); len(m) > 1 {
		numStr := strings.ReplaceAll(m[1], ",", ".")
		if val, err := strconv.ParseFloat(numStr, 64); err == nil && val > 0 {
			salary = val * 1000
			hasSalary = true
			return
		}
	}

	// C. Format angka bertitik: "8.000.000", "Rp 10.000.000"
	dotRegex := regexp.MustCompile(`(?:rp\.?\s*)?(\d{1,3}(?:\.\d{3}){1,3})\b`)
	if m := dotRegex.FindStringSubmatch(lower); len(m) > 1 {
		numStr := strings.ReplaceAll(m[1], ".", "")
		if val, err := strconv.ParseFloat(numStr, 64); err == nil && val > 0 {
			salary = val
			hasSalary = true
			return
		}
	}

	// D. Angka murni: "5000000" (minimal 5 digit untuk budget)
	plainRegex := regexp.MustCompile(`\b(\d{5,12})\b`)
	if m := plainRegex.FindStringSubmatch(lower); len(m) > 1 {
		if val, err := strconv.ParseFloat(m[1], 64); err == nil && val > 0 {
			salary = val
			hasSalary = true
			return
		}
	}

	return
}

// detectExpenseCategory checks if the message mentions a known expense category.
func detectExpenseCategory(msg string) string {
	lower := strings.ToLower(msg)
	categories := []struct {
		keywords []string
		category string
	}{
		{[]string{"makanan", "makan", "kuliner", "food"}, domain.CategoryFood},
		{[]string{"transportasi", "transport", "ojol", "bensin", "parkir"}, domain.CategoryTransport},
		{[]string{"hiburan", "entertainment", "game", "nonton", "bioskop"}, domain.CategoryEntertainment},
		{[]string{"kesehatan", "obat", "dokter", "health", "medis"}, domain.CategoryHealth},
		{[]string{"belanja", "shopping", "groceries"}, domain.CategoryShopping},
		{[]string{"tagihan", "bill", "listrik", "air", "wifi", "pulsa"}, domain.CategoryBill},
		{[]string{"amal", "sedekah", "infaq", "donasi", "charity"}, domain.CategoryCharity},
		{[]string{"lainnya"}, domain.CategoryOther},
	}
	for _, item := range categories {
		for _, kw := range item.keywords {
			if strings.Contains(lower, kw) {
				return item.category
			}
		}
	}
	return ""
}

var idMonthNames = []string{
	"", "Januari", "Februari", "Maret", "April", "Mei", "Juni",
	"Juli", "Agustus", "September", "Oktober", "November", "Desember",
}

var expenseYearRegex = regexp.MustCompile(`\b(20\d{2})\b`)

// ParseExpenseTargetPeriod extracts the target year, month, and formatted period string from message.
// Defaults to current month if not specified.
func ParseExpenseTargetPeriod(lowerMsg string, now time.Time) (year int, month int, periodStr string) {
	year = now.Year()
	month = int(now.Month())

	// 1. Cek tahun spesifik (contoh: 2025, 2026)
	if match := expenseYearRegex.FindStringSubmatch(lowerMsg); len(match) > 1 {
		if y, err := strconv.Atoi(match[1]); err == nil && y >= 2000 && y <= 2100 {
			year = y
		}
	}

	// 2. Cek frasa relatif: bulan lalu / kemarin / last month
	if strings.Contains(lowerMsg, "bulan lalu") ||
		strings.Contains(lowerMsg, "bln lalu") ||
		strings.Contains(lowerMsg, "bulan kemarin") ||
		strings.Contains(lowerMsg, "bln kemarin") ||
		strings.Contains(lowerMsg, "last month") {
		prev := now.AddDate(0, -1, 0)
		year = prev.Year()
		month = int(prev.Month())
	} else if strings.Contains(lowerMsg, "2 bulan lalu") || strings.Contains(lowerMsg, "dua bulan lalu") {
		prev := now.AddDate(0, -2, 0)
		year = prev.Year()
		month = int(prev.Month())
	} else if strings.Contains(lowerMsg, "hari ini") || strings.Contains(lowerMsg, "today") {
		year = now.Year()
		month = int(now.Month())
		periodStr = fmt.Sprintf("Hari ini (%02d %s %d)", now.Day(), idMonthNames[int(now.Month())], now.Year())
		return year, month, periodStr
		
	} else {
		// 3. Cek nama bulan eksplisit (Januari - Desember)
		monthMap := []struct {
			m        int
			patterns []string
		}{
			{1, []string{"januari", "january", "jan"}},
			{2, []string{"februari", "february", "feb"}},
			{3, []string{"maret", "march", "mar"}},
			{4, []string{"april", "apr"}},
			{5, []string{"mei", "may"}},
			{6, []string{"juni", "june", "jun"}},
			{7, []string{"juli", "july", "jul"}},
			{8, []string{"agustus", "august", "agt", "ags", "aug"}},
			{9, []string{"september", "sept", "sep"}},
			{10, []string{"oktober", "october", "okt", "oct"}},
			{11, []string{"november", "nov"}},
			{12, []string{"desember", "december", "des", "dec"}},
		}

		foundMonth := 0
		for _, entry := range monthMap {
			for _, pat := range entry.patterns {
				re := regexp.MustCompile(`\b` + regexp.QuoteMeta(pat) + `\b`)
				if re.MatchString(lowerMsg) {
					foundMonth = entry.m
					break
				}
			}
			if foundMonth > 0 {
				break
			}
		}

		if foundMonth > 0 {
			month = foundMonth
			hasExplicitYear := expenseYearRegex.MatchString(lowerMsg)
			if !hasExplicitYear && month > int(now.Month()) {
				year = now.Year() - 1
			}
		}
	}

	periodStr = fmt.Sprintf("%s %d", idMonthNames[month], year)
	return year, month, periodStr
}

// handleSearch performs a web search and directly returns Tavily's synthesized answer.
func (u *chatUsecase) handleSearch(ctx context.Context, message string) (string, error) {
	results, err := u.searchClient.Search(ctx, message, maxSearchResults)
	if err != nil {
		u.logger.Error("search failed, falling back to LLM direct answer", zap.Error(err))
		return u.handleGeneralChat(ctx, message)
	}

	if len(results) == 0 {
		fmt.Println("No results found, falling back to LLM direct answer")
		return u.handleGeneralChat(ctx, message)
	}

	var directAnswer string
	var sources []string

	for _, r := range results {
		if r.Title == "Tavily Answer" {
			directAnswer = r.Snippet
		} else if r.URL != "" {
			sources = append(sources, fmt.Sprintf("• %s\n  %s", r.Title, r.URL))
		}
	}

	var sb strings.Builder
	if directAnswer != "" {
		sb.WriteString(directAnswer)
	} else if len(results) > 0 {
		sb.WriteString(results[0].Snippet)
	}

	if len(sources) > 0 {
		sb.WriteString("\n\n*Sumber Referensi:*\n")
		sb.WriteString(strings.Join(sources, "\n"))
	}

	return strings.TrimSpace(sb.String()), nil
}

// handleGeneralChat handles general conversation with context history and long-term personality memory.
func (u *chatUsecase) handleGeneralChat(ctx context.Context, message string) (string, error) {
	// handling assistant memory into his master
	lower := strings.ToLower(strings.TrimSpace(message))
	if strings.Contains(lower, "kamu tau apa") || strings.Contains(lower, "kamu tahu apa") ||
		strings.Contains(lower, "inget apa tentang aku") || strings.Contains(lower, "ingat apa tentang aku") ||
		strings.Contains(lower, "daftar memori") || strings.Contains(lower, "list memori") {
		return u.handleListMemories(ctx)
	}

	// Get chat history, so the assistant didn't hallucinate and know the context of conversation
	history, err := u.chatRepo.GetRecentMessages(ctx, maxChatHistory)
	if err != nil {
		u.logger.Warn("failed to get chat history", zap.Error(err))
	}

	messages := make([]domain.ChatMessage, 0, len(history)+2)

	// Inject Long-Term Memories as System Prompt Context
	if u.memoryRepo != nil {
		memories, err := u.memoryRepo.GetAll(ctx)
		if err == nil && len(memories) > 0 {
			var sb strings.Builder
			sb.WriteString("Fakta & kepribadian user yang sudah kamu pelajari dari percakapan sebelumnya:\n")
			for _, m := range memories {
				sb.WriteString(fmt.Sprintf("- [%s] %s\n", m.Category, m.Fact))
			}
			sb.WriteString("\nGunakan fakta ini secara natural — bersikap seolah kamu memang sudah kenal dia lama. JANGAN sebut frasa 'berdasarkan memori' atau 'saya ingat bahwa'.")
			messages = append(messages, domain.ChatMessage{
				Role:    domain.RoleSystem,
				Message: sb.String(),
			})
		}
	}

	for _, h := range history {
		messages = append(messages, *h)
	}

	// Make sure the latest message in the edge of message array.
	// (saveMessage called before handleGeneralChat, so history already contains this message.
	// But if the history is empty or trimmed, we make sure the message is still there.)
	if len(messages) == 0 || messages[len(messages)-1].Message != message {
		messages = append(messages, domain.ChatMessage{
			Role:    domain.RoleUser,
			Message: message,
		})
	}

	return u.llm.Chat(ctx, messages)
}

func (u *chatUsecase) handleListMemories(ctx context.Context) (string, error) {
	if u.memoryRepo == nil {
		return "Belum ada memori yang tersimpan wir.", nil
	}
	memories, err := u.memoryRepo.GetAll(ctx)
	if err != nil || len(memories) == 0 {
		return "Belum ada catatan kepribadian atau kebiasaan lu yang gw simpen wir. Sering-sering ngobrol ya biar gw makin kenal! 😉", nil
	}

	var sb strings.Builder
	sb.WriteString("🧠 *Hal-hal yang udah gw pahami & ingat tentang lu sejauh ini:*\n\n")
	for i, m := range memories {
		categoryIcon := "📌"
		switch m.Category {
		case domain.MemoryCategoryPersonality:
			categoryIcon = "🎭"
		case domain.MemoryCategoryPreference:
			categoryIcon = "⭐"
		case domain.MemoryCategoryHabit:
			categoryIcon = "⏰"
		case domain.MemoryCategoryWork:
			categoryIcon = "💼"
		case domain.MemoryCategoryPersonal:
			categoryIcon = "👤"
		}
		sb.WriteString(fmt.Sprintf("%d. %s *[%s]* %s\n", i+1, categoryIcon, strings.ToUpper(m.Category), m.Fact))
	}
	sb.WriteString("\n_Catatan ini otomatis bertambah/update saat lu ngobrol santai!_")
	return sb.String(), nil
}

// analyzeMemoriesAsync extracts facts/traits in the background without slowing down chat responses.
func (u *chatUsecase) analyzeMemoriesAsync(userMessage string) {
	if u.memoryRepo == nil || u.llm == nil {
		return
	}

	clean := strings.TrimSpace(userMessage)
	if len(strings.Fields(clean)) < 3 {
		return
	}

	lower := strings.ToLower(clean)
	skipKeywords := []string{"menu", "fitur", "batal", "cancel", "ya", "oke", "yes", "sip", "koreksi", "hapus", "reset"}
	for _, kw := range skipKeywords {
		if lower == kw {
			return
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	items, err := u.llm.ExtractMemories(ctx, clean)
	if err != nil {
		u.logger.Debug("extract memories skipped or failed", zap.Error(err))
		return
	}

	for _, item := range items {
		factClean := strings.TrimSpace(item.Fact)
		if factClean == "" {
			continue
		}

		similar, err := u.memoryRepo.FindSimilar(ctx, factClean)
		if err != nil || similar {
			continue
		}

		cat := item.Category
		if cat == "" {
			cat = domain.MemoryCategoryPersonal
		}

		mem := &domain.Memory{
			Category: cat,
			Fact:     factClean,
		}
		if err := u.memoryRepo.Save(ctx, mem); err != nil {
			u.logger.Warn("failed to save user memory", zap.Error(err))
		} else {
			u.logger.Info("🧠 Memory learned automatically!",
				zap.String("category", mem.Category),
				zap.String("fact", mem.Fact),
			)
		}
	}
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
