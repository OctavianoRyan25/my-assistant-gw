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

// Mock LLM specifically for testing audio tool calling
type mockAudioLLM struct {
	transcribeResult *domain.IntentResult
	transcribeErr    error
	toolCallsToReturn []domain.ToolCall
	convReplyToReturn string
	toolCallsErr     error
	chatReply        string
}

func (m *mockAudioLLM) Chat(ctx context.Context, messages []domain.ChatMessage) (string, error) {
	return m.chatReply, nil
}
func (m *mockAudioLLM) ClassifyIntent(ctx context.Context, message string) (*domain.IntentResult, error) {
	return &domain.IntentResult{Intent: domain.IntentGeneralChat, Confidence: 0.5}, nil
}
func (m *mockAudioLLM) ClassifyImageIntent(ctx context.Context, imageData []byte, mimeType string) (*domain.IntentResult, error) {
	return nil, nil
}
func (m *mockAudioLLM) ClassifyAudioIntent(ctx context.Context, audioData []byte, mimeType string) (*domain.IntentResult, error) {
	if m.transcribeErr != nil {
		return nil, m.transcribeErr
	}
	return m.transcribeResult, nil
}
func (m *mockAudioLLM) ParseReminder(ctx context.Context, message string) (string, string, *string, error) {
	return "", "", nil, nil
}
func (m *mockAudioLLM) ParseReminders(ctx context.Context, message string) ([]domain.ReminderParsed, error) {
	return nil, nil
}
func (m *mockAudioLLM) ParseExpense(ctx context.Context, message string) (float64, string, string, error) {
	return 0, "", "", nil
}
func (m *mockAudioLLM) ParseReceiptFromImage(ctx context.Context, imageData []byte, mimeType string) (*domain.ReceiptParsed, error) {
	return nil, nil
}
func (m *mockAudioLLM) ExtractMemories(ctx context.Context, conversation string) ([]domain.MemoryItem, error) {
	return nil, nil
}
func (m *mockAudioLLM) ParseToolCalls(ctx context.Context, message string) ([]domain.ToolCall, string, error) {
	if m.toolCallsErr != nil {
		return nil, "", m.toolCallsErr
	}
	return m.toolCallsToReturn, m.convReplyToReturn, nil
}

// Mock Reminder Usecase
type mockAudioReminderUC struct {
	createdTitle string
	createdTime  time.Time
}

func (m *mockAudioReminderUC) CreateReminder(ctx context.Context, title string, scheduledAt time.Time, recurrence *string) (*domain.Reminder, error) {
	m.createdTitle = title
	m.createdTime = scheduledAt
	return &domain.Reminder{
		ID:          101,
		Title:       title,
		ScheduledAt: scheduledAt,
		Recurrence:  recurrence,
	}, nil
}
func (m *mockAudioReminderUC) ListActive(ctx context.Context) ([]*domain.Reminder, error) {
	return nil, nil
}
func (m *mockAudioReminderUC) Cancel(ctx context.Context, id int64) error { return nil }
func (m *mockAudioReminderUC) Delete(ctx context.Context, id int64) (*domain.Reminder, error) {
	return nil, nil
}
func (m *mockAudioReminderUC) DeleteLatest(ctx context.Context) (*domain.Reminder, error) {
	return nil, nil
}
func (m *mockAudioReminderUC) Update(ctx context.Context, id int64, title string, scheduledAt time.Time) (*domain.Reminder, error) {
	return nil, nil
}
func (m *mockAudioReminderUC) ProcessDueReminders(ctx context.Context) error { return nil }

// Mock Workout Usecase
type mockAudioWorkoutUC struct {
	recordedWorkout *domain.Workout
}

func (m *mockAudioWorkoutUC) RecordWorkout(ctx context.Context, workout *domain.Workout) (*domain.Workout, error) {
	m.recordedWorkout = workout
	workout.ID = 202
	return workout, nil
}
func (m *mockAudioWorkoutUC) RecordWorkoutFromText(ctx context.Context, message string) (*domain.Workout, error) {
	return nil, nil
}
func (m *mockAudioWorkoutUC) RecordWorkoutFromImage(ctx context.Context, imageData []byte, mimeType, caption string) (*domain.Workout, error) {
	return nil, nil
}
func (m *mockAudioWorkoutUC) GetMonthlyStats(ctx context.Context, year, month int) (*domain.WorkoutMonthlyStats, error) {
	return nil, nil
}
func (m *mockAudioWorkoutUC) GetCurrentMonthStats(ctx context.Context) (*domain.WorkoutMonthlyStats, error) {
	return nil, nil
}
func (m *mockAudioWorkoutUC) GetRecentWorkouts(ctx context.Context, limit int) ([]*domain.Workout, error) {
	return nil, nil
}
func (m *mockAudioWorkoutUC) DeleteWorkout(ctx context.Context, id int64) (*domain.Workout, error) {
	return nil, nil
}
func (m *mockAudioWorkoutUC) DeleteLatestWorkout(ctx context.Context) (*domain.Workout, error) {
	return nil, nil
}
func (m *mockAudioWorkoutUC) SendMonthlyReportIfDue(ctx context.Context) error { return nil }

type mockAudioExpenseUC struct {
	mockExpenseUC
	lastRecordedAmount float64
	lastRecordedCat    string
	lastRecordedDesc   string
}

func (m *mockAudioExpenseUC) RecordExpense(ctx context.Context, amount float64, category, description string, occurredAt time.Time) (*domain.Expense, error) {
	m.lastRecordedAmount = amount
	m.lastRecordedCat = category
	m.lastRecordedDesc = description
	return &domain.Expense{
		ID:          303,
		Amount:      amount,
		Category:    category,
		Description: description,
		OccurredAt:  occurredAt,
	}, nil
}

func TestHandleAudioMessage_MultiIntentToolCalling(t *testing.T) {
	tz := time.FixedZone("WIB", 7*3600)
	logger := zap.NewNop()

	t.Run("multi-intent voice note executes all tools", func(t *testing.T) {
		mockLLM := &mockAudioLLM{
			transcribeResult: &domain.IntentResult{
				Intent:     domain.IntentGeneralChat,
				Confidence: 0.9,
				RawMessage: "tadi makan nasi padang 700 kalori seharga 35 ribu, ingetin nanti jam 4 sore meeting tim, dan tadi jogging 30 menit 200 kalori",
			},
			toolCallsToReturn: []domain.ToolCall{
				{
					Name: "log_meal",
					Args: map[string]interface{}{
						"food_name": "Nasi Padang",
						"calories":  700,
						"notes":     "1 porsi",
					},
				},
				{
					Name: "record_expense",
					Args: map[string]interface{}{
						"amount":      35000,
						"category":    "Makanan",
						"description": "Nasi padang siang",
					},
				},
				{
					Name: "create_reminder",
					Args: map[string]interface{}{
						"title":        "Meeting tim",
						"scheduled_at": "2026-10-30 16:00",
					},
				},
				{
					Name: "record_workout",
					Args: map[string]interface{}{
						"title":            "Jogging",
						"type":             "running",
						"duration_minutes": 30,
						"calories_burned":  200,
					},
				},
			},
			convReplyToReturn: "Semua beres dicatat ya wir! Semangat buat meetingnya!",
		}

		mockNutrition := &calorieMockNutritionUC{
			summary: &domain.DailyNutritionSummary{
				TargetCalories: 2000,
				TotalCalories:  700,
			},
		}
		mockExpense := &mockAudioExpenseUC{}
		mockReminder := &mockAudioReminderUC{}
		mockWorkout := &mockAudioWorkoutUC{}

		chatUC := usecase.NewChatUsecase(
			&mockChatRepo{},
			nil,
			mockLLM,
			nil,
			mockReminder,
			mockExpense,
			mockNutrition,
			mockWorkout,
			nil,
			nil,
			logger,
			tz,
		)

		reply, err := chatUC.HandleAudioMessage(context.Background(), []byte("fake-ogg-audio"), "audio/ogg")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify transcript header
		if !strings.Contains(reply, "Pesan Suara Terproses") {
			t.Errorf("expected transcript header, got: %s", reply)
		}
		// Verify meal confirmation
		if !strings.Contains(reply, "Nasi Padang") || !strings.Contains(reply, "700 kcal") {
			t.Errorf("expected meal log in reply, got: %s", reply)
		}
		// Verify expense confirmation
		if !strings.Contains(reply, "35.000") {
			t.Errorf("expected expense log in reply, got: %s", reply)
		}
		// Verify reminder confirmation
		if !strings.Contains(reply, "Meeting tim") {
			t.Errorf("expected reminder log in reply, got: %s", reply)
		}
		// Verify workout confirmation
		if !strings.Contains(reply, "Jogging") || !strings.Contains(reply, "30 menit") {
			t.Errorf("expected workout log in reply, got: %s", reply)
		}
		// Verify conversational response
		if !strings.Contains(reply, "Semua beres dicatat ya wir!") {
			t.Errorf("expected conversational response, got: %s", reply)
		}

		// Verify state changes
		if mockNutrition.lastLoggedFood != "Nasi Padang" || mockNutrition.lastLoggedCal != 700 {
			t.Errorf("nutrition usecase not called correctly: %+v", mockNutrition)
		}
		if mockReminder.createdTitle != "Meeting tim" {
			t.Errorf("reminder usecase not called correctly: %+v", mockReminder)
		}
		if mockWorkout.recordedWorkout == nil || mockWorkout.recordedWorkout.Title != "Jogging" {
			t.Errorf("workout usecase not called correctly: %+v", mockWorkout)
		}
	})

	t.Run("inaudible audio returns warning", func(t *testing.T) {
		mockLLM := &mockAudioLLM{
			transcribeResult: &domain.IntentResult{
				Intent:     domain.IntentGeneralChat,
				Confidence: 0.3,
				RawMessage: "Ora krungu njir, suaramu alon.",
			},
		}

		chatUC := usecase.NewChatUsecase(
			&mockChatRepo{},
			nil,
			mockLLM,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			logger,
			tz,
		)

		reply, err := chatUC.HandleAudioMessage(context.Background(), []byte("noise-audio"), "audio/ogg")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(reply, "Ora krungu") {
			t.Errorf("expected inaudible audio reply, got: %s", reply)
		}
	})

	t.Run("general chat voice note returns natural response", func(t *testing.T) {
		mockLLM := &mockAudioLLM{
			transcribeResult: &domain.IntentResult{
				Intent:     domain.IntentGeneralChat,
				Confidence: 0.95,
				RawMessage: "halo bro gimana kabarmu hari ini",
			},
			toolCallsToReturn: []domain.ToolCall{}, // no tools
			convReplyToReturn: "Aman wir! Lu gimana? Ada yang mau dibantu hari ini?",
		}

		chatUC := usecase.NewChatUsecase(
			&mockChatRepo{},
			nil,
			mockLLM,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			logger,
			tz,
		)

		reply, err := chatUC.HandleAudioMessage(context.Background(), []byte("hello-audio"), "audio/ogg")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(reply, "Aman wir! Lu gimana?") {
			t.Errorf("expected chat reply, got: %s", reply)
		}
	})
}
