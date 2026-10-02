package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
)

const openRouterBaseURL = "https://openrouter.ai/api/v1"

// openRouterClient implements domain.LLMClient using OpenRouter API.
type openRouterClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewOpenRouterClient creates a new OpenRouter LLM client.
func NewOpenRouterClient(apiKey, model string) domain.LLMClient {
	return &openRouterClient{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// --- Internal request/response structs ---

type orMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type orRequest struct {
	Model    string      `json:"model"`
	Messages []orMessage `json:"messages"`
}

type orResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// --- LLMClient implementation ---

func (c *openRouterClient) Chat(ctx context.Context, messages []domain.ChatMessage) (string, error) {
	orMessages := make([]orMessage, 0, len(messages)+1)

	// Add system prompt
	orMessages = append(orMessages, orMessage{
		Role: "system",
		Content: `Kamu adalah teman ngobrol tongkrongan/grup FB, BUKAN asisten virtual formal, BUKAN customer service.

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

Waktu server: ` + time.Now().Format("Monday, 02 January 2006 15:04 WIB"),
	})

	for _, msg := range messages {
		orMessages = append(orMessages, orMessage{
			Role:    msg.Role,
			Content: msg.Message,
		})
	}

	return c.callAPI(ctx, orMessages)
}

func (c *openRouterClient) ClassifyIntent(ctx context.Context, message string) (*domain.IntentResult, error) {
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

	resp, err := c.callAPI(ctx, []orMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: message},
	})
	if err != nil {
		return nil, err
	}

	// Parse JSON response
	resp = strings.TrimSpace(resp)
	// Sometimes LLM wraps in markdown code block
	resp = strings.TrimPrefix(resp, "```json")
	resp = strings.TrimPrefix(resp, "```")
	resp = strings.TrimSuffix(resp, "```")
	resp = strings.TrimSpace(resp)

	var result domain.IntentResult
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		// Fallback to general chat if parsing fails
		return &domain.IntentResult{Intent: domain.IntentGeneralChat, Confidence: 0.5, RawMessage: message}, nil
	}
	result.RawMessage = message
	return &result, nil
}

// normalizeRecurrence ensures "null" and "" strings become nil.
func normalizeRecurrence(r *string) *string {
	if r != nil && (*r == "null" || *r == "") {
		return nil
	}
	return r
}

// ParseReminder parses one or multiple reminders from a natural language message.
// It handles both a single JSON object and a JSON array returned by the LLM.
func (c *openRouterClient) ParseReminder(ctx context.Context, message string) (title string, scheduledAt string, recurrence *string, err error) {
	reminders, err := c.ParseReminders(ctx, message)
	if err != nil || len(reminders) == 0 {
		return "", "", nil, err
	}
	// Caller only expects one — return the first
	first := reminders[0]
	return first.Title, first.ScheduledAt, first.Recurrence, nil
}

// ParseReminders parses one or more reminders from a natural language message.
// Returns a slice so callers can create all requested reminders at once.
func (c *openRouterClient) ParseReminders(ctx context.Context, message string) ([]domain.ReminderParsed, error) {
	now := time.Now()
	systemPrompt := fmt.Sprintf(`Kamu adalah parser reminder. Ekstrak informasi reminder dari pesan pengguna.
Waktu sekarang: %s (WIB, UTC+7)

Jika ada SATU reminder, kembalikan JSON object:
{"title": "<judul>", "scheduled_at": "<YYYY-MM-DD HH:MM:SS>", "recurrence": "<daily|weekly|monthly|null>"}

Jika ada BEBERAPA reminder, kembalikan JSON array:
[
  {"title": "<judul1>", "scheduled_at": "<YYYY-MM-DD HH:MM:SS>", "recurrence": "<daily|weekly|monthly|null>"},
  {"title": "<judul2>", "scheduled_at": "<YYYY-MM-DD HH:MM:SS>", "recurrence": "<daily|weekly|monthly|null>"}
]

Aturan:
- scheduled_at harus dalam format YYYY-MM-DD HH:MM:SS
- Konversi waktu relatif ("besok", "3 jam lagi", "tiap Senin jam 9") dengan benar
- recurrence: isi sesuai pola berulang, atau null jika sekali jalan
- Kalau tidak ada jam yang disebutkan, default jam 08:00
- Kembalikan HANYA JSON, tanpa penjelasan apapun`, now.Format("Monday, 02 January 2006 15:04:05"))

	resp, err := c.callAPI(ctx, []orMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: message},
	})
	if err != nil {
		return nil, err
	}

	return parseRemindersFromJSON(resp)
}

func (c *openRouterClient) ParseExpense(ctx context.Context, message string) (amount float64, category string, description string, err error) {
	systemPrompt := `Kamu adalah parser pengeluaran. Ekstrak informasi pengeluaran dari pesan pengguna.
Kembalikan HANYA JSON valid:
{"amount": <angka dalam rupiah>, "category": "<kategori>", "description": "<deskripsi singkat>"}

Kategori yang tersedia: makan, transport, hiburan, kesehatan, belanja, tagihan, lainnya
Aturan:
- amount harus angka (bukan string), dalam rupiah. "50rb" = 50000, "1.5jt" = 1500000
- Infer kategori dari konteks jika tidak disebutkan eksplisit`

	resp, err := c.callAPI(ctx, []orMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: message},
	})
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
		return 0, "", "", fmt.Errorf("parse expense JSON: %w (raw: %s)", err, resp)
	}

	// Handle amount as various types
	switch v := parsed.Amount.(type) {
	case float64:
		amount = v
	case string:
		amount, err = strconv.ParseFloat(strings.ReplaceAll(v, ",", ""), 64)
		if err != nil {
			return 0, "", "", fmt.Errorf("parse expense amount: %w", err)
		}
	}

	return amount, parsed.Category, parsed.Description, nil
}

// callAPI sends a request to OpenRouter using the provided context and returns the text content.
func (c *openRouterClient) callAPI(ctx context.Context, messages []orMessage) (string, error) {
	reqBody := orRequest{
		Model:    c.model,
		Messages: messages,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("openrouter marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", openRouterBaseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("openrouter create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://github.com/OctavianoRyan25/my-assistant-gw")
	req.Header.Set("X-Title", "Personal Assistant GW")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openrouter http call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("openrouter read body: %w", err)
	}

	var orResp orResponse
	if err := json.Unmarshal(body, &orResp); err != nil {
		return "", fmt.Errorf("openrouter unmarshal response: %w", err)
	}
	if orResp.Error != nil {
		return "", fmt.Errorf("openrouter API error: %s", orResp.Error.Message)
	}
	if len(orResp.Choices) == 0 {
		return "", fmt.Errorf("openrouter: no choices in response")
	}
	return orResp.Choices[0].Message.Content, nil
}

func cleanJSONResponse(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// ParseReceiptFromImage is not supported by OpenRouter client (no vision support in this implementation).
// Switch to Gemini provider (LLM_PROVIDER=gemini) to enable receipt scanning.
func (c *openRouterClient) ParseReceiptFromImage(_ context.Context, _ []byte, _ string) (*domain.ReceiptParsed, error) {
	return nil, fmt.Errorf("fitur scan struk tidak tersedia saat menggunakan OpenRouter. Gunakan provider Gemini (LLM_PROVIDER=gemini) untuk mengaktifkan fitur ini")
}

func (c *openRouterClient) ClassifyAudioIntent(_ context.Context, _ []byte, _ string) (*domain.IntentResult, error) {
	return nil, fmt.Errorf("fitur voice note tidak tersedia saat menggunakan OpenRouter. Gunakan provider Gemini (LLM_PROVIDER=gemini) untuk mengaktifkan fitur ini")
}
