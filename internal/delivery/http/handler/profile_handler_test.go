package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/delivery/http/handler"
	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type mockProfileUC struct {
	profile *domain.UserProfile
}

func (m *mockProfileUC) GetProfile(ctx context.Context) (*domain.UserProfile, error) {
	if m.profile == nil {
		m.profile = &domain.UserProfile{
			HeightCM:           175,
			WeightKG:           70,
			Age:                27,
			Gender:             domain.GenderMale,
			ActivityLevel:      domain.ActivityModeratelyActive,
			DailyCalorieTarget: 2200,
			MonthlySalary:      8000000,
		}
	}
	return m.profile, nil
}

func (m *mockProfileUC) UpdatePhysicalMetrics(ctx context.Context, heightCM, weightKG float64, age int, gender, activityLevel string) (*domain.UserProfile, error) {
	p, _ := m.GetProfile(ctx)
	if heightCM > 0 {
		p.HeightCM = heightCM
	}
	if weightKG > 0 {
		p.WeightKG = weightKG
	}
	if age > 0 {
		p.Age = age
	}
	if gender != "" {
		p.Gender = gender
	}
	if activityLevel != "" {
		p.ActivityLevel = activityLevel
	}
	return p, nil
}

func (m *mockProfileUC) SetSalary(ctx context.Context, monthlySalary float64) (*domain.UserProfile, error) {
	p, _ := m.GetProfile(ctx)
	p.MonthlySalary = monthlySalary
	return p, nil
}

func (m *mockProfileUC) CalculateBMRAndTDEE(profile *domain.UserProfile) (float64, float64) {
	return 1650, 2200
}

func TestProfileHandler_Get(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profile", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	h := handler.NewProfileHandler(&mockProfileUC{}, nil, zap.NewNop())
	if err := h.Get(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["success"] != true {
		t.Errorf("expected success true, got %v", resp["success"])
	}
}

func TestProfileHandler_Update(t *testing.T) {
	e := echo.New()
	body := `{"height_cm": 180, "weight_kg": 75, "age": 28, "gender": "male", "activity_level": "very_active", "monthly_salary": 10000000}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/profile", bytes.NewBufferString(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	h := handler.NewProfileHandler(&mockProfileUC{}, nil, zap.NewNop())
	if err := h.Update(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

