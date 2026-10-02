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
	systemPrompt := `Kamu adalah teman ngobrol tongkrongan/grup FB, BUKAN asisten virtual formal, BUKAN customer service.

ATURAN UTAMA:
- HAPUS SEMUA GAYA BICARA FORMAL. Dilarang pakai: "Tentu", "Halo", "Ada yang bisa dibantu?", "Berikut adalah", "Semoga membantu".
- Langsung to the point, jangan basa-basi kayak bot kantor.
- Bahasa: Kasual, santai, bahasa tongkrongan/FB. Boleh pakai sapaan "wir", "wak", "le", "cik", "njir", atau nyebut diri "gw"/"gwejh" secara wajar.
- Jawab pendek & padat layaknya chat WhatsApp/FB Messenger antar temen.

CONTOH YANG BENAR:
- User: "Catat pengeluaran kopi 25rb"
  Bot: "Beres wir, udah gw catet 25rb buat kopi 🐱"
- User: "Besok ada agenda apa?"
  Bot: "Jadwal lo padet bgt njir besok, ada miting jam 10 sama jam 2 siang loh ya."

Waktu server: ` + time.Now().Format("Monday, 02 January 2006 15:04 WIB")

	contents := make([]geminiContent, 0, len(messages))
	for _, msg := range messages {
		if strings.TrimSpace(msg.Message) == "" {
			continue // skip pesan kosong agar Gemini tidak error INVALID_ARGUMENT
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

	reqBody := geminiRequest{
		SystemInstruction: &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		},
		Contents: contents,
	}

	return c.callAPI(ctx, reqBody)
}

// ClassifyIntent uses Gemini to classify user intent.
func (c *geminiClient) ClassifyIntent(ctx context.Context, message string) (*domain.IntentResult, error) {
	systemPrompt := `Kamu adalah classifier intent. Analisa pesan pengguna dan tentukan intent-nya.
Kembalikan HANYA JSON valid dengan format:
{"intent": "<intent>", "confidence": <0.0-1.0>}

Intent yang tersedia:
- "reminder": user ingin membuat, melihat, mengubah, atau membatalkan reminder/jadwal
- "expense": user ingin mencatat pengeluaran, atau menanyakan laporan pengeluaran
- "search": user menanyakan informasi terkini, berita, atau hal yang butuh info real-time
- "reset": user ingin mereset/menghapus riwayat percakapan
- "general_chat": percakapan umum, brainstorming, diskusi, atau hal lainnya

Contoh:
- "ingatkan aku besok jam 8 meeting" → {"intent": "reminder", "confidence": 0.98}
- "keluar 50rb buat makan" → {"intent": "expense", "confidence": 0.97}
- "berita AI hari ini apa?" → {"intent": "search", "confidence": 0.95}
- "mulai obrolan baru" → {"intent": "reset", "confidence": 0.99}
- "kamu bisa apa?" → {"intent": "general_chat", "confidence": 0.90}`

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

func (c *geminiClient) ClassifyAudioIntent(ctx context.Context, audioData []byte, mimeType string) (*domain.IntentResult, error) {
	// WhatsApp voice notes umumnya bertipe audio/ogg codecs=opus
	if mimeType == "" {
		mimeType = "audio/ogg"
	}

	systemPrompt := `Kamu adalah classifier intent. Dengarkan rekaman pesan suara pengguna, transkrip isi ucapannya, lalu analisa intent-nya.
Kembalikan HANYA JSON valid dengan format:
{"intent": "<intent>", "confidence": <0.0-1.0>, "raw_message": "<transkrip isi ucapan pengguna>"}

Intent yang tersedia:
- "reminder": user ingin membuat, melihat, mengubah, atau membatalkan reminder/jadwal
- "expense": user ingin mencatat pengeluaran, atau menanyakan laporan pengeluaran
- "search": user menanyakan informasi terkini, berita, atau hal yang butuh info real-time
- "reset": user ingin mereset/menghapus riwayat percakapan
- "general_chat": percakapan umum, brainstorming, diskusi, atau hal lainnya

Contoh:
- (audio: "ingatkan aku besok jam 8 meeting") → {"intent": "reminder", "confidence": 0.98, "raw_message": "ingatkan aku besok jam 8 meeting"}
- (audio: "keluar 50rb buat makan") → {"intent": "expense", "confidence": 0.97, "raw_message": "keluar 50rb buat makan"}
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
