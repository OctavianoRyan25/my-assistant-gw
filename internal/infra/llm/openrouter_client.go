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

// ExtractMemories analyzes a user message or conversation turn and extracts newly discovered long-term facts or traits.
func (c *openRouterClient) ExtractMemories(ctx context.Context, conversation string) ([]domain.MemoryItem, error) {
	systemPrompt := `Kamu adalah asisten analisis memori kepribadian.
Tugasmu: Analisis pesan/percakapan berikut dari pengguna (user).
Cari apakah ada fakta permanen, kepribadian, gaya komunikasi, preferensi (makanan, minuman, hobi, teknologi), kebiasaan, pekerjaan, atau info personal jangka panjang tentang dirinya.

ATURAN PENTING:
1. JANGAN simpan hal-hal sementara/situasional (misal: "lagi macet", "lapar", "besok meeting", "mau tidur", "lagi nonton").
2. Simpan HANYA fakta atau preferensi bernilai jangka panjang yang mencerminkan profil/kepribadian/kebiasaan pengguna.
3. Kategori yang valid: "personality", "preference", "habit", "work", "personal".
4. Jika TIDAK ADA fakta permanen/profil baru yang relevan, kembalikan JSON array KOSONG: []

Kembalikan HANYA JSON array valid tanpa markdown formatting. Format:
[
  {
    "category": "personality|preference|habit|work|personal",
    "fact": "Deskripsi fakta singkat, jelas, dan baku"
  }
]`

	messages := []orMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: conversation},
	}

	raw, err := c.callAPI(ctx, messages)
	if err != nil {
		return nil, err
	}

	raw = cleanJSONResponse(raw)
	var items []domain.MemoryItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("openrouter unmarshal memories: %w (raw: %s)", err, raw)
	}
	return items, nil
}

func (c *openRouterClient) ClassifyIntent(ctx context.Context, message string) (*domain.IntentResult, error) {
	systemPrompt := `Kamu adalah classifier intent untuk AI asisten personal. Tugasmu: baca pesan pengguna (dan konteks percakapan sebelumnya jika ada), tentukan intent pesan TERAKHIR user, kembalikan JSON.
Kembalikan HANYA JSON valid:
{"intent": "<intent>", "confidence": <0.0-1.0>}

Intent yang tersedia:
- "reminder": user ingin MEMBUAT, MELIHAT DAFTAR, MENGUBAH, atau MENGHAPUS reminder/jadwal/alarm
- "expense": user ingin MENCATAT pengeluaran uang BARU, MELIHAT DAFTAR/LIST pengeluaran, MELIHAT laporan/rekap, atau menanyakan total biaya pengeluaran
- "workout": user MELAPORKAN aktivitas olahraga yang sudah/baru dilakukan, atau ingin MELIHAT/MENGHAPUS riwayat workout
- "search": user menanyakan informasi terkini, berita, atau hal yang butuh info real-time
- "reset": user ingin mereset/menghapus riwayat percakapan
- "general_chat": percakapan umum, brainstorming, diskusi, atau hal lainnya

ATURAN PENTING:
1. Fokus pada pesan terbaru user. Konteks hanya untuk memahami referensi ambigu.
2. Pesan singkat seperti "oke", "ya", "sip", "wkwk", "haha" → "general_chat" confidence tinggi (≥0.90).
3. "expense" mencakup DUA sub-kasus:
   a. CATAT pengeluaran: ada nominal uang eksplisit ("keluar 50rb", "beli kopi 25000", "bayar ojol 15rb")
   b. LIHAT/LIST pengeluaran: "list pengeluaran", "daftar pengeluaran", "lihat pengeluaran", "riwayat pengeluaran", "laporan pengeluaran", "rekap pengeluaran", "berapa pengeluaran"
   KEDUANYA harus di-route ke "expense" — handler akan membedakan sendiri.
4. HANYA "workout" jika user melaporkan aktivitas SUDAH dilakukan, atau mau LIHAT/HAPUS data workout.
5. Kalau ragu, pilih confidence lebih rendah dan pertimbangkan "general_chat".

Contoh:
- "ingatkan aku besok jam 8 meeting" → {"intent": "reminder", "confidence": 0.98}
- "list reminder" / "daftar reminder" → {"intent": "reminder", "confidence": 0.97}
- "keluar 50rb buat makan" → {"intent": "expense", "confidence": 0.97}
- "list pengeluaran" → {"intent": "expense", "confidence": 0.97}
- "daftar pengeluaran" → {"intent": "expense", "confidence": 0.97}
- "lihat pengeluaran" → {"intent": "expense", "confidence": 0.96}
- "rekap pengeluaran" → {"intent": "expense", "confidence": 0.96}
- "laporan pengeluaran bulan ini" → {"intent": "expense", "confidence": 0.96}
- "habis gym 60 menit, push day" → {"intent": "workout", "confidence": 0.97}
- "list workout" → {"intent": "workout", "confidence": 0.98}
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
	systemPrompt := `Kamu adalah parser pengeluaran. Tugasmu HANYA mengekstrak data pencatatan pengeluaran baru dari pesan pengguna.

Kembalikan HANYA JSON valid:
{"amount": <angka dalam rupiah>, "category": "<kategori>", "description": "<deskripsi singkat>"}

Kategori yang tersedia: makan, transport, hiburan, kesehatan, belanja, tagihan, lainnya

ATURAN PENTING:
- amount harus angka (bukan string), dalam rupiah. "50rb" = 50000, "1.5jt" = 1500000, "25k" = 25000
- Infer kategori dari konteks jika tidak disebutkan eksplisit
- JIKA pesan pengguna adalah permintaan MELIHAT/LIST/REKAP pengeluaran (bukan mencatat baru), kembalikan: {"amount": 0, "category": "", "description": "LIST_REQUEST"}
- JIKA pesan tidak mengandung nominal pengeluaran yang jelas, kembalikan: {"amount": 0, "category": "", "description": ""}
- JANGAN mengarang nominal jika tidak ada dalam pesan

Contoh BENAR (mencatat):
- "keluar 50rb buat makan" → {"amount": 50000, "category": "makan", "description": "makan"}
- "beli kopi 25000" → {"amount": 25000, "category": "makan", "description": "kopi"}
- "bayar ojol 15rb" → {"amount": 15000, "category": "transport", "description": "ojol"}

Contoh BUKAN pencatatan (kembalikan amount: 0):
- "list pengeluaran" → {"amount": 0, "category": "", "description": "LIST_REQUEST"}
- "daftar pengeluaran" → {"amount": 0, "category": "", "description": "LIST_REQUEST"}
- "berapa pengeluaran gw" → {"amount": 0, "category": "", "description": "LIST_REQUEST"}`

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

func (c *openRouterClient) ClassifyImageIntent(ctx context.Context, imageData []byte, mimeType string) (*domain.IntentResult, error) {
	return  nil, fmt.Errorf("fitur image tidak tersedia saat menggunakan OpenRouter. Gunakan provider Gemini (LLM_PROVIDER=gemini) untuk mengaktifkan fitur ini")
}

