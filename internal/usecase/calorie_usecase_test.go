package usecase_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
	"github.com/OctavianoRyan25/my-assistant-gw/internal/usecase"
	"go.uber.org/zap"
)

type mockChatRepo struct{}

func (m *mockChatRepo) SaveMessage(ctx context.Context, msg *domain.ChatMessage) error { return nil }
func (m *mockChatRepo) GetRecentMessages(ctx context.Context, limit int) ([]*domain.ChatMessage, error) {
	return nil, nil
}
func (m *mockChatRepo) ClearHistory(ctx context.Context) error { return nil }

type mockChatLLM struct {
	intentToReturn string
	confidence     float64
	chatReply      string
}

func (m *mockChatLLM) Chat(ctx context.Context, messages []domain.ChatMessage) (string, error) {
	return m.chatReply, nil
}
func (m *mockChatLLM) ClassifyIntent(ctx context.Context, message string) (*domain.IntentResult, error) {
	return &domain.IntentResult{
		Intent:     m.intentToReturn,
		Confidence: m.confidence,
	}, nil
}
func (m *mockChatLLM) ClassifyImageIntent(ctx context.Context, imageData []byte, mimeType string) (*domain.IntentResult, error) {
	return nil, nil
}
func (m *mockChatLLM) ClassifyAudioIntent(ctx context.Context, audioData []byte, mimeType string) (*domain.IntentResult, error) {
	return nil, nil
}
func (m *mockChatLLM) ParseReminder(ctx context.Context, message string) (string, string, *string, error) {
	return "", "", nil, nil
}
func (m *mockChatLLM) ParseReminders(ctx context.Context, message string) ([]domain.ReminderParsed, error) {
	return nil, nil
}
func (m *mockChatLLM) ParseExpense(ctx context.Context, message string) (float64, string, string, error) {
	return 0, "", "", nil
}
func (m *mockChatLLM) ParseReceiptFromImage(ctx context.Context, imageData []byte, mimeType string) (*domain.ReceiptParsed, error) {
	return nil, nil
}
func (m *mockChatLLM) ExtractMemories(ctx context.Context, conversation string) ([]domain.MemoryItem, error) {
	return nil, nil
}

type calorieMockNutritionUC struct {
	summary       *domain.DailyNutritionSummary
	lastLoggedFood string
	lastLoggedCal  int
	lastLoggedNote string
	deletedID      int64
	latestDeleted  bool
}

func (m *calorieMockNutritionUC) LogMealFromImage(ctx context.Context, imageData []byte, mimeType string, notes string) (*domain.FoodLog, *domain.DailyNutritionSummary, error) {
	return nil, nil, nil
}

func (m *calorieMockNutritionUC) LogMealFromText(ctx context.Context, foodName string, calories int, notes string, loggedAt time.Time) (*domain.FoodLog, *domain.DailyNutritionSummary, error) {
	m.lastLoggedFood = foodName
	m.lastLoggedCal = calories
	m.lastLoggedNote = notes
	food := &domain.FoodLog{
		ID:                10,
		FoodName:          foodName,
		EstimatedCalories: calories,
		Notes:             notes,
		LoggedAt:          loggedAt,
	}
	return food, m.summary, nil
}

func (m *calorieMockNutritionUC) GetDailySummary(ctx context.Context, date time.Time) (*domain.DailyNutritionSummary, error) {
	return m.summary, nil
}

func (m *calorieMockNutritionUC) DeleteMeal(ctx context.Context, id int64) error {
	m.deletedID = id
	return nil
}

func (m *calorieMockNutritionUC) DeleteLatestMeal(ctx context.Context) (*domain.FoodLog, error) {
	m.latestDeleted = true
	return &domain.FoodLog{
		ID:                99,
		FoodName:          "Nasi Uduk",
		EstimatedCalories: 450,
	}, nil
}

func TestParseMealTextInput(t *testing.T) {
	tests := []struct {
		input        string
		expectedFood string
		expectedCal  int
		expectedNote string
		expectedOK   bool
	}{
		{
			input:        "catat kalori nasi padang 700 kcal",
			expectedFood: "Nasi Padang",
			expectedCal:  700,
			expectedNote: "",
			expectedOK:   true,
		},
		{
			input:        "tadi siang makan ayam geprek 550 kalori catatan sambal bawang level 5",
			expectedFood: "Ayam Geprek",
			expectedCal:  550,
			expectedNote: "sambal bawang level 5",
			expectedOK:   true,
		},
		{
			input:        "sarapan bubur ayam 320 kal",
			expectedFood: "Bubur Ayam",
			expectedCal:  320,
			expectedNote: "",
			expectedOK:   true,
		},
		{
			input:        "makan nasi uduk 450 kcal note tanpa kerupuk",
			expectedFood: "Nasi Uduk",
			expectedCal:  450,
			expectedNote: "tanpa kerupuk",
			expectedOK:   true,
		},
		{
			input:        "sate ayam 400 kalori",
			expectedFood: "Sate Ayam",
			expectedCal:  400,
			expectedNote: "",
			expectedOK:   true,
		},
		{
			input:        "halo apa kabar",
			expectedFood: "",
			expectedCal:  0,
			expectedNote: "",
			expectedOK:   false,
		},
		{
			input:        "makan nasi padang tanpa kalori",
			expectedFood: "",
			expectedCal:  0,
			expectedNote: "",
			expectedOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			food, cal, notes, ok := usecase.ParseMealTextInput(tt.input)
			if ok != tt.expectedOK {
				t.Fatalf("expected ok=%v, got %v for input: %s", tt.expectedOK, ok, tt.input)
			}
			if ok {
				if food != tt.expectedFood {
					t.Errorf("expected food=%s, got %s", tt.expectedFood, food)
				}
				if cal != tt.expectedCal {
					t.Errorf("expected cal=%d, got %d", tt.expectedCal, cal)
				}
				if notes != tt.expectedNote {
					t.Errorf("expected notes=%s, got %s", tt.expectedNote, notes)
				}
			}
		})
	}
}

func TestFormatDailyNutritionSummary(t *testing.T) {
	tz := time.FixedZone("WIB", 7*3600)
	now := time.Now().In(tz)

	t.Run("with meals logged", func(t *testing.T) {
		summary := &domain.DailyNutritionSummary{
			Date:              now,
			TargetCalories:    2000,
			TotalCalories:     1250,
			RemainingCalories: 750,
			Status:            domain.CalorieStatusOnTrack,
			TotalProtein:      65,
			TotalCarbs:        150,
			TotalFat:          40,
			Advice:            "Asupan kalori masih aman!",
			Meals: []*domain.FoodLog{
				{
					ID:                1,
					FoodName:          "Nasi Padang Rendang",
					EstimatedCalories: 750,
					Notes:             "1 porsi",
					LoggedAt:          now,
				},
				{
					ID:                2,
					FoodName:          "Ayam Bakar",
					EstimatedCalories: 500,
					LoggedAt:          now.Add(3 * time.Hour),
				},
			},
		}

		formatted := usecase.FormatDailyNutritionSummary(summary, tz)
		if !strings.Contains(formatted, "Status & Rekap Kalori Hari Ini") {
			t.Errorf("expected header, got: %s", formatted)
		}
		if !strings.Contains(formatted, "*1250 kcal* / Target: *2000 kcal*") {
			t.Errorf("expected total calories, got: %s", formatted)
		}
		if !strings.Contains(formatted, "Nasi Padang Rendang") {
			t.Errorf("expected meal name, got: %s", formatted)
		}
		if !strings.Contains(formatted, "750 kcal") {
			t.Errorf("expected meal calories, got: %s", formatted)
		}
	})

	t.Run("nil summary", func(t *testing.T) {
		formatted := usecase.FormatDailyNutritionSummary(nil, tz)
		if !strings.Contains(formatted, "Belum ada catatan kalori") {
			t.Errorf("expected empty message, got: %s", formatted)
		}
	})
}

func TestHandleCalorieInChatUsecase(t *testing.T) {
	tz := time.FixedZone("WIB", 7*3600)
	logger := zap.NewNop()

	t.Run("view calorie summary", func(t *testing.T) {
		mockNutrition := &calorieMockNutritionUC{
			summary: &domain.DailyNutritionSummary{
				TargetCalories:    2000,
				TotalCalories:     850,
				RemainingCalories: 1150,
				Status:            domain.CalorieStatusOnTrack,
			},
		}
		mockLLM := &mockChatLLM{
			intentToReturn: domain.IntentCalorie,
			confidence:     0.95,
		}

		chatUC := usecase.NewChatUsecase(
			&mockChatRepo{},
			nil,
			mockLLM,
			nil,
			nil,
			nil,
			mockNutrition,
			nil,
			nil,
			nil,
			logger,
			tz,
		)

		reply, err := chatUC.HandleMessage(context.Background(), "total kalori hari ini")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(reply, "Status & Rekap Kalori Hari Ini") {
			t.Errorf("expected summary reply, got: %s", reply)
		}
		if !strings.Contains(reply, "850 kcal") {
			t.Errorf("expected total calories, got: %s", reply)
		}
	})

	t.Run("log meal with regex", func(t *testing.T) {
		mockNutrition := &calorieMockNutritionUC{
			summary: &domain.DailyNutritionSummary{
				TargetCalories:    2000,
				TotalCalories:     700,
				RemainingCalories: 1300,
			},
		}
		mockLLM := &mockChatLLM{
			intentToReturn: domain.IntentCalorie,
			confidence:     0.95,
		}

		chatUC := usecase.NewChatUsecase(
			&mockChatRepo{},
			nil,
			mockLLM,
			nil,
			nil,
			nil,
			mockNutrition,
			nil,
			nil,
			nil,
			logger,
			tz,
		)

		reply, err := chatUC.HandleMessage(context.Background(), "catat kalori nasi padang 700 kcal")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(reply, "Makanan Tercatat") {
			t.Errorf("expected meal confirmation, got: %s", reply)
		}
		if mockNutrition.lastLoggedFood != "Nasi Padang" {
			t.Errorf("expected food Nasi Padang, got: %s", mockNutrition.lastLoggedFood)
		}
		if mockNutrition.lastLoggedCal != 700 {
			t.Errorf("expected 700 kcal, got: %d", mockNutrition.lastLoggedCal)
		}
	})

	t.Run("log meal with LLM estimation", func(t *testing.T) {
		mockNutrition := &calorieMockNutritionUC{
			summary: &domain.DailyNutritionSummary{
				TargetCalories:    2000,
				TotalCalories:     400,
				RemainingCalories: 1600,
			},
		}
		mockLLM := &mockChatLLM{
			intentToReturn: domain.IntentCalorie,
			confidence:     0.95,
			chatReply:      `{"food_name": "Sate Ayam", "estimated_calories": 400, "notes": "10 tusuk"}`,
		}

		chatUC := usecase.NewChatUsecase(
			&mockChatRepo{},
			nil,
			mockLLM,
			nil,
			nil,
			nil,
			mockNutrition,
			nil,
			nil,
			nil,
			logger,
			tz,
		)

		reply, err := chatUC.HandleMessage(context.Background(), "tadi makan sate ayam 10 tusuk")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(reply, "Makanan Tercatat") {
			t.Errorf("expected meal confirmation, got: %s", reply)
		}
		if mockNutrition.lastLoggedFood != "Sate Ayam" {
			t.Errorf("expected food Sate Ayam, got: %s", mockNutrition.lastLoggedFood)
		}
		if mockNutrition.lastLoggedCal != 400 {
			t.Errorf("expected 400 kcal, got: %d", mockNutrition.lastLoggedCal)
		}
	})

	t.Run("delete meal", func(t *testing.T) {
		mockNutrition := &calorieMockNutritionUC{}
		mockLLM := &mockChatLLM{
			intentToReturn: domain.IntentCalorie,
			confidence:     0.95,
		}

		chatUC := usecase.NewChatUsecase(
			&mockChatRepo{},
			nil,
			mockLLM,
			nil,
			nil,
			nil,
			mockNutrition,
			nil,
			nil,
			nil,
			logger,
			tz,
		)

		reply, err := chatUC.HandleMessage(context.Background(), "hapus makanan terakhir")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(reply, "berhasil dihapus") {
			t.Errorf("expected delete reply, got: %s", reply)
		}
		if !mockNutrition.latestDeleted {
			t.Errorf("expected latestDeleted to be true")
		}
	})
}
