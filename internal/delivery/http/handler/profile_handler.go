package handler

import (
	"net/http"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// ProfileHandler handles HTTP endpoints for user profile management.
type ProfileHandler struct {
	profileUC domain.UserProfileUsecase
	budgetUC  domain.BudgetUsecase
	logger    *zap.Logger
}

// NewProfileHandler creates a new ProfileHandler.
func NewProfileHandler(profileUC domain.UserProfileUsecase, budgetUC domain.BudgetUsecase, logger *zap.Logger) *ProfileHandler {
	return &ProfileHandler{
		profileUC: profileUC,
		budgetUC:  budgetUC,
		logger:    logger,
	}
}

type updateProfileRequest struct {
	HeightCM      float64 `json:"height_cm"`
	WeightKG      float64 `json:"weight_kg"`
	Age           int     `json:"age"`
	Gender        string  `json:"gender"`
	ActivityLevel string  `json:"activity_level"`
	MonthlySalary float64 `json:"monthly_salary"`
}

type profileResponse struct {
	Profile *domain.UserProfile `json:"profile"`
	BMR     float64             `json:"bmr"`
	TDEE    float64             `json:"tdee"`
}

// Get returns the current user profile with BMR and TDEE calculations.
// GET /api/v1/profile
func (h *ProfileHandler) Get(c echo.Context) error {
	ctx := c.Request().Context()
	profile, err := h.profileUC.GetProfile(ctx)
	if err != nil {
		h.logger.Error("failed to get user profile", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, echo.Map{"success": false, "error": err.Error()})
	}

	bmr, tdee := h.profileUC.CalculateBMRAndTDEE(profile)
	return c.JSON(http.StatusOK, echo.Map{
		"success": true,
		"data": profileResponse{
			Profile: profile,
			BMR:     bmr,
			TDEE:    tdee,
		},
	})
}

// Update updates the user physical metrics and monthly salary.
// PUT /api/v1/profile
func (h *ProfileHandler) Update(c echo.Context) error {
	var req updateProfileRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"success": false, "error": "format data tidak valid"})
	}

	ctx := c.Request().Context()

	// Update physical metrics
	_, err := h.profileUC.UpdatePhysicalMetrics(ctx, req.HeightCM, req.WeightKG, req.Age, req.Gender, req.ActivityLevel)
	if err != nil {
		h.logger.Error("failed to update user physical metrics", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, echo.Map{"success": false, "error": err.Error()})
	}

	// Update monthly salary / budget if provided
	if req.MonthlySalary >= 0 {
		if h.budgetUC != nil {
			_, err = h.budgetUC.SetBudget(ctx, req.MonthlySalary, domain.DefaultWarningThresholdPercent)
			if err != nil {
				h.logger.Warn("failed to sync budget with new salary", zap.Error(err))
			}
		} else {
			_, _ = h.profileUC.SetSalary(ctx, req.MonthlySalary)
		}
	}

	// Fetch refreshed profile
	profile, err := h.profileUC.GetProfile(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"success": false, "error": err.Error()})
	}

	bmr, tdee := h.profileUC.CalculateBMRAndTDEE(profile)
	return c.JSON(http.StatusOK, echo.Map{
		"success": true,
		"message": "Profil berhasil diperbarui",
		"data": profileResponse{
			Profile: profile,
			BMR:     bmr,
			TDEE:    tdee,
		},
	})
}

