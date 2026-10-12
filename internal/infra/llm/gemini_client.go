package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
)

const geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"

type geminiClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewGeminiClient creates a domain.LLMClient powered directly by Google Gemini API.
func NewGeminiClient(apiKey, model string) domain.LLMClient {
	if model == "" {
		model = "gemini-1.5-flash"
	}
	return &geminiClient{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

// Gemini API data structures

// geminiInlineData carries raw bytes (e.g. an image) encoded as base64.
type geminiInlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"` // base64-encoded bytes
}

// geminiPart is flexible: it may carry text OR inlineData (for vision calls).
type geminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inline_data,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"` // "user" or "model"
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	Temperature      *float64 `json:"temperature,omitempty"`
	ResponseMimeType string   `json:"response_mime_type,omitempty"` // "application/json"
}

type geminiRequest struct {
	SystemInstruction *geminiContent          `json:"system_instruction,omitempty"`
	Contents          []geminiContent         `json:"contents"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
			Role string `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

// Chat sends conversation history to Gemini and returns the assistant reply.
func (c *geminiClient) Chat(ctx context.Context, messages []domain.ChatMessage) (string, error) {
	systemPrompt := `Kamu adalah AI asisten personal bernama "GW-AI" dengan kepribadian teman tongkrongan — kasual, nyablak, dan jujur.

KEPRIBADIAN & GAYA BICARA:
- Bicara seperti teman deket via WhatsApp. Pakai bahasa gaul wajar: "wir", "njir", "cik", "wak", "le", nyebut diri "gw".
- JANGAN formal. JANGAN pakai: "Tentu saja", "Halo", "Ada yang bisa dibantu?", "Semoga membantu", "Berikut adalah".
- Jawab pendek dan to the point. Hanya panjang kalau konteks memang perlu detail.
- Emoji boleh, tapi jangan lebay.
- Kalau user nanya sesuatu, jawab dulu pertanyaannya — jangan malah tanya balik atau ngalih topik.
- Kalau konteks percakapan sebelumnya relevan, gunakan — jangan pura-pura lupa.

KONTEKS PERCAKAPAN:
- Baca seluruh history percakapan sebelum menjawab. Kalau user bilang "itu", "tadi", "yang itu", "lanjutin" — cari referensinya di history.
- Jangan jawab seolah-olah ini percakapan baru kalau sebelumnya sudah ada konteks.
- Kalau user nanya hal yang sudah pernah dibahas, kasih jawaban yang konsisten.

PENTING — JANGAN HALUSINASI:
- Kamu di sini hanya untuk OBROLAN UMUM. Sistem lain yang sudah mengeksekusi perintah seperti catat workout, buat reminder, catat pengeluaran, dll.
- JANGAN bilang "udah gw catet", "udah gw hapus", dsb. kecuali memang dalam konteks konfirmasi yang sudah ditetapkan sistem.
- Kalau user tanya kapabilitas atau minta sesuatu di luar kemampuanmu, bilang jujur dengan gaya santai.
- Jangan jawab terlalu panjang untuk pesan singkat.

WAKTU: ` + time.Now().Format("Monday, 02 January 2006 15:04 WIB")

	var extraSystem []string
	contents := make([]geminiContent, 0, len(messages))
	for _, msg := range messages {
		if strings.TrimSpace(msg.Message) == "" {
			continue // skip pesan kosong agar Gemini tidak error INVALID_ARGUMENT
		}
		if msg.Role == domain.RoleSystem {
			extraSystem = append(extraSystem, msg.Message)
			continue
		}
		role := "user"
		if msg.Role == domain.RoleAssistant {
			role = "model"
		}
		contents = append(contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: msg.Message}},
		})
	}

	if len(extraSystem) > 0 {
		systemPrompt += "\n\n" + strings.Join(extraSystem, "\n\n")
	}

	temp := 0.7
	reqBody := geminiRequest{
		SystemInstruction: &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		},
		Contents: contents,
		GenerationConfig: &geminiGenerationConfig{
			Temperature: &temp,
		},
	}

	return c.callAPI(ctx, reqBody)
}

// ExtractMemories analyzes a user message or conversation turn and extracts newly discovered long-term facts or traits.
func (c *geminiClient) ExtractMemories(ctx context.Context, conversation string) ([]domain.MemoryItem, error) {
	systemPrompt := `Kamu adalah asisten analisis memori kepribadian.
Tugasmu: Analisis pesan/percakapan berikut dari pengguna (user).
Cari apakah ada fakta permanen, kepribadian, gaya komunikasi, preferensi (makanan, minuman, hobi, teknologi), kebiasaan, pekerjaan, atau info personal jangka panjang tentang dirinya.

ATURAN PENTING:
1. JANGAN simpan hal-hal sementara/situasional (misal: "lagi macet", "lapar", "besok meeting", "mau tidur", "lagi nonton").
2. Simpan HANYA fakta atau preferensi bernilai jangka panjang yang mencerminkan profil/kepribadian/kebiasaan pengguna.
3. Contoh yang HARUS diekstrak:
   - "gw suka kopi tanpa gula" -> category: "preference", fact: "Menyukai kopi hitam tanpa gula"
   - "gw ga suka bertele-tele" -> category: "personality", fact: "Menyukai komunikasi to-the-point dan tidak bertele-tele"
   - "gw biasa begadang sampe subuh" -> category: "habit", fact: "Punya kebiasaan tidur larut malam / begadang"
   - "gw software engineer backend golang" -> category: "work", fact: "Bekerja sebagai backend software engineer dengan Go (Golang)"
   - "panggil gw Ryan" -> category: "personal", fact: "Nama panggilan Ryan"
4. Kategori yang valid: "personality", "preference", "habit", "work", "personal".
5. Jika TIDAK ADA fakta permanen/profil baru yang relevan, kembalikan JSON array KOSONG: []

Kembalikan HANYA JSON array valid tanpa markdown formatting. Format:
[
  {
    "category": "personality|preference|habit|work|personal",
    "fact": "Deskripsi fakta singkat, jelas, dan baku"
  }
]`

	reqBody := geminiRequest{
		SystemInstruction: &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		},
		Contents: []geminiContent{
			{
				Role:  "user",
				Parts: []geminiPart{{Text: conversation}},
			},
		},
		GenerationConfig: &geminiGenerationConfig{
			ResponseMimeType: "application/json",
		},
	}

	resp, err := c.callAPI(ctx, reqBody)
	if err != nil {
		return nil, err
	}

	resp = cleanJSONResponse(resp)
	var items []domain.MemoryItem
	if err := json.Unmarshal([]byte(resp), &items); err != nil {
		return nil, fmt.Errorf("gemini unmarshal memories: %w (raw: %s)", err, resp)
	}
	return items, nil
}

// ClassifyIntent uses Gemini to classify user intent.
// message may include a "CONTEXT:" prefix with recent history prepended by the caller.
func (c *geminiClient) ClassifyIntent(ctx context.Context, message string) (*domain.IntentResult, error) {
	systemPrompt := `Kamu adalah classifier intent untuk AI asisten personal. Tugasmu: baca pesan pengguna (dan konteks percakapan sebelumnya jika ada), tentukan intent pesan TERAKHIR user, kembalikan JSON.
Kembalikan HANYA JSON valid:
{"intent": "<intent>", "confidence": <0.0-1.0>}

Intent yang tersedia:
- "reminder": user ingin MEMBUAT, MELIHAT, MENGUBAH, atau MENGHAPUS reminder/jadwal/alarm
- "expense": user ingin MENCATAT pengeluaran uang, MELIHAT laporan pengeluaran, atau menanyakan total biaya
- "workout": user MELAPORKAN aktivitas olahraga yang sudah/baru dilakukan, atau ingin MELIHAT/MENGHAPUS riwayat workout
- "search": user menanyakan informasi terkini, berita, fakta, atau hal yang butuh data real-time dari internet
- "reset": user ingin menghapus/mereset seluruh riwayat percakapan
- "general_chat": obrolan umum, pertanyaan konsep, brainstorming, curhat, diskusi, tindak lanjut dari obrolan sebelumnya, atau hal yang tidak masuk kategori di atas

ATURAN PENTING:
1. Fokus pada pesan terbaru user (bagian "PESAN TERBARU:"). Konteks hanya digunakan untuk memahami referensi ambigu.
2. Pesan singkat seperti "oke", "ya", "sip", "lanjut", "terus?", "wkwk", "haha", "emang", "iya bener" → "general_chat" confidence tinggi (≥0.90). JANGAN route ke handler spesifik hanya karena topik sebelumnya adalah reminder/expense/workout.
3. "hapus"/"batal"/"cancel" + konteks reminder/jadwal/alarm → "reminder"
4. "hapus"/"batal"/"cancel" + konteks workout/olahraga/gym/lari → "workout"
5. HANYA "workout" jika user benar-benar melaporkan aktivitas yang SUDAH dilakukan, atau mau LIHAT/HAPUS data workout. Rencana ("besok mau gym") → "general_chat".
6. HANYA "expense" jika ada nominal uang EKSPLISIT atau kata kunci pengeluaran yang sangat jelas.
7. Kalau ragu antara dua intent, pilih yang confidence-nya lebih rendah dan pertimbangkan "general_chat".

Contoh:
- "ingatkan aku besok jam 8 meeting" → {"intent": "reminder", "confidence": 0.98}
- "daftar reminder" → {"intent": "reminder", "confidence": 0.97}
- "hapus reminder 5" → {"intent": "reminder", "confidence": 0.98}
- "batalkan reminder meeting besok" → {"intent": "reminder", "confidence": 0.96}
- "hapus reminder terakhir" → {"intent": "reminder", "confidence": 0.97}
- "keluar 50rb buat makan" → {"intent": "expense", "confidence": 0.97}
- "laporan pengeluaran bulan ini" → {"intent": "expense", "confidence": 0.95}
- "habis gym 60 menit, push day" → {"intent": "workout", "confidence": 0.97}
- "tadi jogging 5km 40 menit" → {"intent": "workout", "confidence": 0.96}
- "list workout" → {"intent": "workout", "confidence": 0.98}
- "hapus workout terakhir" → {"intent": "workout", "confidence": 0.97}
- "hapus workout 5" → {"intent": "workout", "confidence": 0.97}
- "statistik workout bulan ini" → {"intent": "workout", "confidence": 0.95}
- "besok mau lari pagi" → {"intent": "general_chat", "confidence": 0.80}
- "berita AI hari ini apa?" → {"intent": "search", "confidence": 0.95}
- "mulai obrolan baru" → {"intent": "reset", "confidence": 0.99}
- "kamu bisa apa?" → {"intent": "general_chat", "confidence": 0.92}
- "haha iya bener" → {"intent": "general_chat", "confidence": 0.95}
- "wkwk anjir" → {"intent": "general_chat", "confidence": 0.97}
- "asik" → {"intent": "general_chat", "confidence": 0.96}
- "gimana menurut kamu?" → {"intent": "general_chat", "confidence": 0.93}
- "terus?" → {"intent": "general_chat", "confidence": 0.92}
- "maksudnya?" → {"intent": "general_chat", "confidence": 0.91}
- "kenapa?" → {"intent": "general_chat", "confidence": 0.90}`

	reqBody := geminiRequest{
		SystemInstruction: &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		},
		Contents: []geminiContent{
			{
				Role:  "user",
				Parts: []geminiPart{{Text: message}},
			},
		},
		GenerationConfig: &geminiGenerationConfig{
			ResponseMimeType: "application/json",
		},
	}

	resp, err := c.callAPI(ctx, reqBody)
	if err != nil {
		return nil, err
	}

	resp = cleanJSONResponse(resp)

	var result domain.IntentResult
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return &domain.IntentResult{Intent: domain.IntentGeneralChat, Confidence: 0.5, RawMessage: message}, nil
	}
	result.RawMessage = message
	return &result, nil
}

// ParseReminder extracts a single reminder (delegates to ParseReminders).
func (c *geminiClient) ParseReminder(ctx context.Context, message string) (title string, scheduledAt string, recurrence *string, err error) {
	reminders, err := c.ParseReminders(ctx, message)
	if err != nil || len(reminders) == 0 {
		return "", "", nil, err
	}
	first := reminders[0]
	return first.Title, first.ScheduledAt, first.Recurrence, nil
}

// rawReminderItem is a flexible struct supporting various JSON key names Gemini might produce.
type rawReminderItem struct {
	Title          string  `json:"title"`
	Name           string  `json:"name"`
	Task           string  `json:"task"`
	ScheduledAt    string  `json:"scheduled_at"`
	ScheduledAtCam string  `json:"scheduledAt"`
	DateTime       string  `json:"datetime"`
	DateTimeSnake  string  `json:"date_time"`
	Time           string  `json:"time"`
	Recurrence     *string `json:"recurrence"`
	Repeat         *string `json:"repeat"`
}

func (r *rawReminderItem) toDomain() (domain.ReminderParsed, bool) {
	title := r.Title
	if title == "" {
		title = r.Name
	}
	if title == "" {
		title = r.Task
	}

	sched := r.ScheduledAt
	if sched == "" {
		sched = r.ScheduledAtCam
	}
	if sched == "" {
		sched = r.DateTime
	}
	if sched == "" {
		sched = r.DateTimeSnake
	}
	if sched == "" {
		sched = r.Time
	}

	rec := normalizeRecurrence(r.Recurrence)
	if rec == nil {
		rec = normalizeRecurrence(r.Repeat)
	}

	if title == "" && sched == "" {
		return domain.ReminderParsed{}, false
	}
	return domain.ReminderParsed{
		Title:       title,
		ScheduledAt: sched,
		Recurrence:  rec,
	}, true
}

// ParseReminders extracts one or more reminders from natural language using Gemini.
func (c *geminiClient) ParseReminders(ctx context.Context, message string) ([]domain.ReminderParsed, error) {
	now := time.Now()
	systemPrompt := fmt.Sprintf(`Kamu adalah parser reminder. Ekstrak informasi reminder dari pesan pengguna.
Waktu sekarang: %s (WIB, UTC+7)

Kembalikan SELALU format JSON array of objects:
[
  {
    "title": "<judul reminder>",
    "scheduled_at": "<YYYY-MM-DD HH:MM:SS>",
    "recurrence": "<daily|weekly|monthly|null>"
  }
]

ATURAN PENTING:
- Kembalikan SELALU dalam array [...] meskipun hanya ada 1 reminder.
- Nama field HARUS PERSIS: "title", "scheduled_at", "recurrence". JANGAN gunakan scheduledAt atau membungkus dengan objek {"reminders": [...]}.
- scheduled_at format YYYY-MM-DD HH:MM:SS (misal: 2026-09-29 18:30:00).
- Konversi waktu relatif ("nanti jam 5 sore", "besok", "tiap senin jam 7 pagi") ke tanggal dan jam spesifik.
- recurrence: isi "daily", "weekly", "monthly", atau null jika bukan jadwal rutin.
- Jika jam tidak disebutkan, default jam 08:00:00.
- Kembalikan HANYA JSON tanpa teks pengantar atau markdown block.`, now.Format("Monday, 02 January 2006 15:04:05"))

	reqBody := geminiRequest{
		SystemInstruction: &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		},
		Contents: []geminiContent{
			{
				Role:  "user",
				Parts: []geminiPart{{Text: message}},
			},
		},
		GenerationConfig: &geminiGenerationConfig{
			ResponseMimeType: "application/json",
		},
	}

	resp, err := c.callAPI(ctx, reqBody)
	if err != nil {
		return nil, err
	}

	return parseRemindersFromJSON(resp)
}

func parseRemindersFromJSON(resp string) ([]domain.ReminderParsed, error) {
	resp = cleanJSONResponse(resp)
	if resp == "" {
		return nil, fmt.Errorf("empty response from LLM")
	}

	// 1. Try directly as array of items: [{...}, {...}]
	var rawArr []rawReminderItem
	if err := json.Unmarshal([]byte(resp), &rawArr); err == nil && len(rawArr) > 0 {
		var results []domain.ReminderParsed
		for _, item := range rawArr {
			if dp, ok := item.toDomain(); ok {
				results = append(results, dp)
			}
		}
		if len(results) > 0 {
			return results, nil
		}
	}

	// 2. Try as object / map (e.g. {"reminders": [...]}, {"reminder": {...}}, etc.)
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(resp), &obj); err == nil {
		wrapperKeys := []string{"reminders", "reminder", "data", "items", "tasks", "list"}
		for _, key := range wrapperKeys {
			if rawVal, found := obj[key]; found {
				// Try as sub-array
				var subArr []rawReminderItem
				if err := json.Unmarshal(rawVal, &subArr); err == nil && len(subArr) > 0 {
					var results []domain.ReminderParsed
					for _, item := range subArr {
						if dp, ok := item.toDomain(); ok {
							results = append(results, dp)
						}
					}
					if len(results) > 0 {
						return results, nil
					}
				}

				// Try as single sub-item
				var subSingle rawReminderItem
				if err := json.Unmarshal(rawVal, &subSingle); err == nil {
					if dp, ok := subSingle.toDomain(); ok {
						return []domain.ReminderParsed{dp}, nil
					}
				}
			}
		}

		// Try the object itself as a single item
		var single rawReminderItem
		if err := json.Unmarshal([]byte(resp), &single); err == nil {
			if dp, ok := single.toDomain(); ok {
				return []domain.ReminderParsed{dp}, nil
			}
		}
	}

	return nil, fmt.Errorf("cannot parse reminder from response (raw: %s)", resp)
}

// ParseExpense extracts structured expense data using Gemini.
func (c *geminiClient) ParseExpense(ctx context.Context, message string) (amount float64, category string, description string, err error) {
	systemPrompt := `Kamu adalah parser pengeluaran. Ekstrak informasi pengeluaran dari pesan pengguna.
Kembalikan HANYA JSON valid:
{"amount": <angka dalam rupiah>, "category": "<kategori>", "description": "<deskripsi singkat>"}

Kategori yang tersedia: makan, transport, hiburan, kesehatan, belanja, tagihan, lainnya
Aturan:
- amount harus angka (bukan string), dalam rupiah. "50rb" = 50000, "1.5jt" = 1500000
- Infer kategori dari konteks jika tidak disebutkan eksplisit`

	reqBody := geminiRequest{
		SystemInstruction: &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		},
		Contents: []geminiContent{
			{
				Role:  "user",
				Parts: []geminiPart{{Text: message}},
			},
		},
		GenerationConfig: &geminiGenerationConfig{
			ResponseMimeType: "application/json",
		},
	}

	resp, err := c.callAPI(ctx, reqBody)
	if err != nil {
		return 0, "", "", err
	}

	resp = cleanJSONResponse(resp)

	var parsed struct {
		Amount      interface{} `json:"amount"`
		Category    string      `json:"category"`
		Description string      `json:"description"`
	}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		return 0, "", "", fmt.Errorf("gemini parse expense JSON: %w (raw: %s)", err, resp)
	}

	switch v := parsed.Amount.(type) {
	case float64:
		amount = v
	case string:
		amount, err = strconv.ParseFloat(strings.ReplaceAll(v, ",", ""), 64)
		if err != nil {
			return 0, "", "", fmt.Errorf("gemini parse expense amount: %w", err)
		}
	case int:
		amount = float64(v)
	default:
		return 0, "", "", fmt.Errorf("gemini parse expense: invalid amount type %T", v)
	}

	return amount, parsed.Category, parsed.Description, nil
}

// ParseReceiptFromImage uses Gemini Vision to extract expense items from a receipt photo.
func (c *geminiClient) ParseReceiptFromImage(ctx context.Context, imageData []byte, mimeType string) (*domain.ReceiptParsed, error) {
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	systemPrompt := `Kamu adalah OCR + parser struk belanja. Analisa gambar struk/nota yang dikirim.

Kembalikan JSON dengan format persis:
{
  "store_name": "<nama toko/merchant, atau empty string jika tidak ada>",
  "items": [
    {"description": "<nama item>", "amount": <harga dalam rupiah, angka>, "category": "<kategori>"}
  ],
  "total": <total belanja dalam rupiah, angka. 0 jika tidak terbaca>,
  "currency": "IDR"
}

Kategori yang tersedia: makan, transport, hiburan, kesehatan, belanja, tagihan, lainnya
Aturan:
- amount dan total harus berupa angka (bukan string), dalam satuan Rupiah.
- Jika ada item yang amount-nya tidak terbaca dengan jelas, estimasikan dari total atau skip item tersebut.
- Jika gambar bukan struk/nota/faktur, kembalikan {"store_name":"","items":[],"total":0,"currency":"IDR"}
- Kembalikan HANYA JSON, tanpa penjelasan apapun.`

	b64 := base64.StdEncoding.EncodeToString(imageData)

	reqBody := geminiRequest{
		SystemInstruction: &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		},
		Contents: []geminiContent{
			{
				Role: "user",
				Parts: []geminiPart{
					{InlineData: &geminiInlineData{MimeType: mimeType, Data: b64}},
					{Text: "Tolong analisa struk ini dan ekstrak semua item beserta harganya."},
				},
			},
		},
		GenerationConfig: &geminiGenerationConfig{
			ResponseMimeType: "application/json",
		},
	}

	resp, err := c.callAPI(ctx, reqBody)
	if err != nil {
		return nil, fmt.Errorf("gemini receipt vision: %w", err)
	}

	resp = cleanJSONResponse(resp)

	var parsed domain.ReceiptParsed
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		return nil, fmt.Errorf("gemini receipt parse JSON: %w (raw: %s)", err, resp)
	}

	// Normalize amounts stored as strings
	for i, item := range parsed.Items {
		if item.Amount == 0 && item.Description != "" {
			// Try to re-parse if mistakenly zero
			parsed.Items[i].Amount = 0
		}
		if parsed.Items[i].Category == "" {
			parsed.Items[i].Category = domain.CategoryOther
		}
	}

	if parsed.Currency == "" {
		parsed.Currency = "IDR"
	}

	return &parsed, nil
}

func (c *geminiClient) ClassifyImageIntent(ctx context.Context, imageData []byte, mimeType string) (*domain.IntentResult, error) {
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	systemPrompt := `Kamu adalah classifier gambar. Analisa gambar yang dikirim pengguna dan tentukan intent-nya.
Kembalikan HANYA JSON valid dengan format:
{"intent": "<intent>", "confidence": <0.0-1.0>}

Intent yang tersedia:
- "calorie": gambar berisi makanan, minuman, atau hidangan yang ingin dihitung kalorinya
- "workout": gambar berisi aktivitas olahraga, screenshot aplikasi fitness/gym, atau bukti workout
- "receipt": gambar berisi struk belanja, nota, faktur, atau bukti pembayaran
- "general_chat": gambar lain yang tidak termasuk kategori di atas

Contoh:
- (gambar nasi goreng) → {"intent": "calorie", "confidence": 0.97}
- (gambar struk Indomaret) → {"intent": "receipt", "confidence": 0.98}
- (screenshot Strava/lari) → {"intent": "workout", "confidence": 0.95}
- (gambar selfie/pemandangan) → {"intent": "general_chat", "confidence": 0.85}`

	b64 := base64.StdEncoding.EncodeToString(imageData)

	reqBody := geminiRequest{
		SystemInstruction: &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		},
		Contents: []geminiContent{
			{
				Role: "user",
				Parts: []geminiPart{
					{InlineData: &geminiInlineData{MimeType: mimeType, Data: b64}},
					{Text: "Klasifikasikan isi gambar ini."},
				},
			},
		},
		GenerationConfig: &geminiGenerationConfig{
			ResponseMimeType: "application/json",
		},
	}

	resp, err := c.callAPI(ctx, reqBody)
	if err != nil {
		return nil, err
	}

	resp = cleanJSONResponse(resp)

	var result domain.IntentResult
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return &domain.IntentResult{Intent: domain.IntentGeneralChat, Confidence: 0.5}, nil
	}
	return &result, nil
}

// ParseMealFromImage uses Gemini Vision to estimate calories and macros from a food photo.
func (c *geminiClient) ParseMealFromImage(ctx context.Context, imageData []byte, mimeType string) (*domain.MealParsed, error) {
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	systemPrompt := `Kamu adalah ahli gizi dan analisis makanan. Analisa gambar makanan yang dikirim dan estimasikan kandungan nutrisinya.

Kembalikan JSON dengan format persis:
{
  "food_name": "<nama makanan/hidangan secara deskriptif>",
  "estimated_calories": <estimasi kalori total dalam kcal, angka bulat>,
  "protein": <estimasi protein dalam gram, 1 desimal>,
  "carbs": <estimasi karbohidrat dalam gram, 1 desimal>,
  "fat": <estimasi lemak dalam gram, 1 desimal>,
  "portion_description": "<deskripsi porsi, contoh: '1 piring nasi + 1 potong ayam goreng'>",
  "confidence": <tingkat keyakinan estimasi 0.0-1.0>,
  "advice": "<saran singkat berkaitan makanan ini dari sudut pandang gizi, max 1 kalimat>"
}

Aturan:
- estimated_calories, protein, carbs, fat harus berupa angka (bukan string).
- Estimasi berdasarkan porsi yang terlihat di gambar (bukan porsi standar).
- Jika gambar BUKAN makanan, kembalikan {"food_name":"","estimated_calories":0,"protein":0,"carbs":0,"fat":0,"portion_description":"","confidence":0,"advice":""}
- Kembalikan HANYA JSON tanpa penjelasan apapun.`

	b64 := base64.StdEncoding.EncodeToString(imageData)

	reqBody := geminiRequest{
		SystemInstruction: &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		},
		Contents: []geminiContent{
			{
				Role: "user",
				Parts: []geminiPart{
					{InlineData: &geminiInlineData{MimeType: mimeType, Data: b64}},
					{Text: "Tolong analisa makanan di gambar ini dan estimasikan kandungan gizinya."},
				},
			},
		},
		GenerationConfig: &geminiGenerationConfig{
			ResponseMimeType: "application/json",
		},
	}

	resp, err := c.callAPI(ctx, reqBody)
	if err != nil {
		return nil, fmt.Errorf("gemini meal vision: %w", err)
	}

	resp = cleanJSONResponse(resp)

	var parsed domain.MealParsed
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		return nil, fmt.Errorf("gemini meal parse JSON: %w (raw: %s)", err, resp)
	}

	if parsed.FoodName == "" || parsed.EstimatedCalories == 0 {
		return nil, fmt.Errorf("gambar tidak mengandung makanan yang dapat dianalisa")
	}

	return &parsed, nil
}

func (c *geminiClient) ClassifyAudioIntent(ctx context.Context, audioData []byte, mimeType string) (*domain.IntentResult, error) {
	// WhatsApp voice notes umumnya bertipe audio/ogg codecs=opus
	if mimeType == "" {
		mimeType = "audio/ogg"
	}

	systemPrompt := `Kamu adalah classifier intent. Dengarkan rekaman pesan suara pengguna, transkrip isi ucapannya, lalu analisa intent-nya.
Kembalikan HANYA JSON valid dengan format:
{"intent": "<intent>", "confidence": <0.0-1.0>, "raw_message": "<transkrip isi ucapan pengguna>"}

Intent yang tersedia:
- "reminder": user ingin membuat, melihat, mengubah, membatalkan, atau menghapus reminder/jadwal
- "expense": user ingin mencatat pengeluaran, atau menanyakan laporan pengeluaran
- "workout": user melaporkan aktivitas olahraga (gym, lari, renang, dll), melihat daftar/riwayat/statistik workout, atau menghapus catatan workout
- "search": user menanyakan informasi terkini, berita, atau hal yang butuh info real-time
- "reset": user ingin mereset/menghapus riwayat percakapan
- "general_chat": percakapan umum, brainstorming, diskusi, atau hal lainnya

Contoh:
- (audio: "ingatkan aku besok jam 8 meeting") → {"intent": "reminder", "confidence": 0.98, "raw_message": "ingatkan aku besok jam 8 meeting"}
- (audio: "hapus reminder 1") → {"intent": "reminder", "confidence": 0.98, "raw_message": "hapus reminder 1"}
- (audio: "keluar 50rb buat makan") → {"intent": "expense", "confidence": 0.97, "raw_message": "keluar 50rb buat makan"}
- (audio: "habis gym 60 menit, push day") → {"intent": "workout", "confidence": 0.97, "raw_message": "habis gym 60 menit, push day"}
- (audio: "jogging 5km tadi pagi") → {"intent": "workout", "confidence": 0.96, "raw_message": "jogging 5km tadi pagi"}
- (audio: "lihat riwayat workout") → {"intent": "workout", "confidence": 0.96, "raw_message": "lihat riwayat workout"}
- (audio: hening/noise/tidak jelas) → {"intent": "general_chat", "confidence": 0.3, "raw_message": "Ora krungu njir, suaramu alon."}

PENTING: raw_message harus transkrip asli ucapan pengguna (bahasa Indonesia, apa adanya), bukan jawaban atau respon kamu. Kalau audio tidak jelas/hening, isi raw_message dengan pesan "Ora krungu njir, suaramu alon." dan set intent ke "general_chat".`

	b64 := base64.StdEncoding.EncodeToString(audioData)

	reqBody := geminiRequest{
		SystemInstruction: &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		},
		Contents: []geminiContent{
			{
				Role: "user",
				Parts: []geminiPart{
					{InlineData: &geminiInlineData{MimeType: mimeType, Data: b64}},
					{Text: "Dengarkan pesan suara ini, transkrip, lalu klasifikasikan intent-nya."},
				},
			},
		},
		GenerationConfig: &geminiGenerationConfig{
			ResponseMimeType: "application/json",
		},
	}

	resp, err := c.callAPI(ctx, reqBody)
	if err != nil {
		return nil, err
	}

	resp = cleanJSONResponse(resp)

	var result domain.IntentResult
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return &domain.IntentResult{Intent: domain.IntentGeneralChat, Confidence: 0.5, RawMessage: "Error parsing audio"}, nil
	}
	return &result, nil
}

// ParseWorkoutFromText extracts structured workout data from natural language using Gemini.
func (c *geminiClient) ParseWorkoutFromText(ctx context.Context, message string) (*domain.WorkoutParsed, error) {
	systemPrompt := `Kamu adalah parser aktivitas olahraga. Ekstrak informasi workout dari pesan pengguna.
Kembalikan HANYA JSON valid dengan format:
{
  "title": "<judul singkat aktivitas, contoh: 'Gym Push Day', 'Jogging Pagi', 'Renang 1000m'>",
  "type": "<gym|jogging|running|swimming|cycling|walking|hiit|other>",
  "duration_minutes": <durasi dalam menit, angka bulat, 0 jika tidak disebutkan>,
  "calories_burned": <estimasi kalori terbakar dalam kcal, angka bulat, 0 jika tidak disebutkan>,
  "distance_meters": <jarak dalam meter, angka desimal, 0 jika tidak relevan>,
  "description": "<deskripsi singkat latihan, termasuk detail set/rep jika disebutkan>"
}

Aturan:
- type harus salah satu dari: gym, jogging, running, swimming, cycling, walking, hiit, other
- duration_minutes, calories_burned, distance_meters harus angka (bukan string)
- Konversi jarak: "5km" = 5000, "500m" = 500
- Estimasi calories_burned jika tidak disebutkan berdasarkan tipe dan durasi (contoh: gym 60 menit ≈ 300 kcal, jogging 5km ≈ 350 kcal)
- JIKA pesan pengguna BUKAN laporan aktivitas olahraga fisik yang dilakukan (misalnya instruksi sistem 'list workout', 'hapus workout', pertanyaan, sapaan, atau obrolan umum), kembalikan JSON dengan title string kosong: {"title": "", "type": ""}
- Kembalikan HANYA JSON tanpa teks pengantar atau markdown block`

	reqBody := geminiRequest{
		SystemInstruction: &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		},
		Contents: []geminiContent{
			{
				Role:  "user",
				Parts: []geminiPart{{Text: message}},
			},
		},
		GenerationConfig: &geminiGenerationConfig{
			ResponseMimeType: "application/json",
		},
	}

	resp, err := c.callAPI(ctx, reqBody)
	if err != nil {
		return nil, fmt.Errorf("gemini parse workout text: %w", err)
	}

	resp = cleanJSONResponse(resp)

	var parsed domain.WorkoutParsed
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		return nil, fmt.Errorf("gemini workout parse JSON: %w (raw: %s)", err, resp)
	}

	if strings.TrimSpace(parsed.Title) == "" {
		return nil, fmt.Errorf("tidak dapat mengekstrak informasi workout dari pesan")
	}
	if parsed.Type == "" {
		parsed.Type = domain.WorkoutTypeOther
	}

	return &parsed, nil
}

// ParseWorkoutFromImage extracts structured workout data from a workout photo or fitness app screenshot.
func (c *geminiClient) ParseWorkoutFromImage(ctx context.Context, imageData []byte, mimeType, caption string) (*domain.WorkoutParsed, error) {
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	prompt := "Analisa gambar workout/olahraga ini dan ekstrak informasi aktivitasnya."
	if caption != "" {
		prompt = fmt.Sprintf("Analisa gambar workout ini. Caption dari pengirim: \"%s\". Ekstrak informasi aktivitasnya.", caption)
	}

	systemPrompt := `Kamu adalah parser aktivitas olahraga berbasis gambar. Analisa gambar yang dikirim (foto gym, screenshot Strava/Nike Run/fitness app, bukti olahraga, dll).
Kembalikan HANYA JSON valid dengan format:
{
  "title": "<judul singkat aktivitas, contoh: 'Gym Push Day', 'Jogging Pagi', 'Renang 1000m'>",
  "type": "<gym|jogging|running|swimming|cycling|walking|hiit|other>",
  "duration_minutes": <durasi dalam menit, angka bulat, 0 jika tidak terbaca>,
  "calories_burned": <kalori terbakar dalam kcal, angka bulat, 0 jika tidak terbaca>,
  "distance_meters": <jarak dalam meter, angka desimal, 0 jika tidak relevan>,
  "description": "<deskripsi singkat aktivitas atau detail yang terlihat di gambar>"
}

Aturan:
- type harus salah satu dari: gym, jogging, running, swimming, cycling, walking, hiit, other
- Baca data langsung dari gambar jika tersedia (screenshot app biasanya menampilkan durasi, kalori, jarak)
- Jika gambar bukan tentang olahraga, kembalikan {"title":"","type":"other","duration_minutes":0,"calories_burned":0,"distance_meters":0,"description":""}
- Kembalikan HANYA JSON tanpa teks pengantar apapun`

	b64 := base64.StdEncoding.EncodeToString(imageData)

	reqBody := geminiRequest{
		SystemInstruction: &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		},
		Contents: []geminiContent{
			{
				Role: "user",
				Parts: []geminiPart{
					{InlineData: &geminiInlineData{MimeType: mimeType, Data: b64}},
					{Text: prompt},
				},
			},
		},
		GenerationConfig: &geminiGenerationConfig{
			ResponseMimeType: "application/json",
		},
	}

	resp, err := c.callAPI(ctx, reqBody)
	if err != nil {
		return nil, fmt.Errorf("gemini workout vision: %w", err)
	}

	resp = cleanJSONResponse(resp)

	var parsed domain.WorkoutParsed
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		return nil, fmt.Errorf("gemini workout image parse JSON: %w (raw: %s)", err, resp)
	}

	if parsed.Title == "" && parsed.DurationMinutes == 0 && parsed.CaloriesBurned == 0 {
		return nil, fmt.Errorf("tidak dapat mendeteksi data workout dari gambar")
	}
	if parsed.Type == "" {
		parsed.Type = domain.WorkoutTypeOther
	}

	return &parsed, nil
}

// callAPI sends a request to Google Gemini API and returns the generated text.
func (c *geminiClient) callAPI(ctx context.Context, reqBody geminiRequest) (string, error) {

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("gemini marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s:generateContent?key=%s", geminiBaseURL, c.model, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("gemini create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini http call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("gemini read body: %w", err)
	}

	var gResp geminiResponse
	if err := json.Unmarshal(body, &gResp); err != nil {
		return "", fmt.Errorf("gemini unmarshal response: %w (raw: %s)", err, string(body))
	}

	if gResp.Error != nil {
		return "", fmt.Errorf("gemini API error (%d - %s): %s", gResp.Error.Code, gResp.Error.Status, gResp.Error.Message)
	}

	if len(gResp.Candidates) == 0 || len(gResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini: empty response candidates")
	}

	return gResp.Candidates[0].Content.Parts[0].Text, nil
}
