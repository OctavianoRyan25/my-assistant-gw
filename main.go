package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/delivery/http/handler"
	"github.com/OctavianoRyan25/my-assistant-gw/internal/infra/config"
	"github.com/OctavianoRyan25/my-assistant-gw/internal/infra/database"
	"github.com/OctavianoRyan25/my-assistant-gw/internal/infra/llm"
	"github.com/OctavianoRyan25/my-assistant-gw/internal/infra/logger"
	"github.com/OctavianoRyan25/my-assistant-gw/internal/infra/scheduler"
	"github.com/OctavianoRyan25/my-assistant-gw/internal/infra/search"
	"github.com/OctavianoRyan25/my-assistant-gw/internal/infra/whatsapp"
	"github.com/OctavianoRyan25/my-assistant-gw/internal/repository"
	"github.com/OctavianoRyan25/my-assistant-gw/internal/routes"
	"github.com/OctavianoRyan25/my-assistant-gw/internal/usecase"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
)

func main() {
	fmt.Println("🤖 Starting Personal Assistant GW...")

	// ── 1. Load config ────────────────────────────────────────────────────────
	cfg, err := config.LoadConfig()
	if err != nil {
		panic(fmt.Sprintf("error loading config: %v", err))
	}

	// ── 2. Load logger ────────────────────────────────────────────────────────
	log, err := logger.NewLogger(*cfg)
	if err != nil {
		panic(fmt.Sprintf("error loading logger: %v", err))
	}
	defer log.Sync() //nolint:errcheck

	// ── 3. Load timezone ─────────────────────────────────────────────────────
	tz, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Warn("invalid timezone, falling back to Asia/Jakarta", zap.Error(err))
		tz, _ = time.LoadLocation("Asia/Jakarta")
	}
	log.Info("timezone loaded", zap.String("tz", tz.String()))

	// ── 4. Load database ─────────────────────────────────────────────────────
	db, err := database.NewDatabase(*cfg)
	if err != nil {
		log.Fatal("error loading database", zap.Error(err))
	}
	defer db.Close()
	log.Info("database connected")

	// ── 5. Repositories ──────────────────────────────────────────────────────
	reminderRepo := repository.NewReminderRepository(db)
	expenseRepo  := repository.NewExpenseRepository(db)
	chatRepo     := repository.NewChatRepository(db)

	// ── 6. Infrastructure clients ─────────────────────────────────────────────
	llmClient    := llm.NewOpenRouterClient(cfg.OpenRouterAPIKey, cfg.OpenRouterModel)
	searchClient := search.NewDuckDuckGoClient()
	// Use background context for WhatsApp client init (signal context not created yet)
	waSender, err := whatsapp.NewWhatsAppClient(context.Background())
	if err != nil {
		log.Fatal("error loading whatsapp", zap.Error(err))
	}
	defer waSender.Close()

	// ── 7. Use cases (business logic) ────────────────────────────────────────
	reminderUC := usecase.NewReminderUsecase(reminderRepo, llmClient, waSender, cfg.WAPhoneNumber, log, tz)
	expenseUC  := usecase.NewExpenseUsecase(expenseRepo, waSender, cfg.WAPhoneNumber, log, tz)
	chatUC     := usecase.NewChatUsecase(chatRepo, llmClient, searchClient, reminderUC, expenseUC, log, tz)

	// ── 8. Register WhatsApp message handler ──────────────────────────────────
	waSender.RegisterMessageHandler(chatUC, cfg.WAPhoneNumber, log)

	// ── 9. Scheduler ─────────────────────────────────────────────────────────
	sched := scheduler.NewScheduler(reminderUC, expenseUC, log, tz)
	sched.Start()
	defer sched.Stop()
	log.Info("scheduler started")

	// ── 9. HTTP server ───────────────────────────────────────────────────────
	e := echo.New()
	e.HideBanner = true

	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogURI:    true,
		LogStatus: true,
		LogMethod: true,
		LogError:  true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			log.Info("request",
				zap.String("method", v.Method),
				zap.String("uri", v.URI),
				zap.Int("status", v.Status),
				zap.Error(v.Error),
			)
			return nil
		},
	}))
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// ── 10. Wire handlers & routes ───────────────────────────────────────────
	handlers := &routes.Handlers{
		WebhookHandler:  handler.NewWebhookHandler(chatUC, log),
		ReminderHandler: handler.NewReminderHandler(reminderUC, log, tz),
		ExpenseHandler:  handler.NewExpenseHandler(expenseUC, log, tz),
	}
	routes.RegisterRoutes(e, handlers)

	// ── 11. Graceful shutdown ─────────────────────────────────────────────────
	port := cfg.AppPort
	if port == "" {
		port = "8080"
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("server starting", zap.String("port", port))
		if err := e.Start(":" + port); err != nil {
			log.Info("server stopped", zap.Error(err))
		}
	}()

	<-ctx.Done()
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.Shutdown(shutdownCtx); err != nil {
		log.Error("failed to shutdown server", zap.Error(err))
	}

	log.Info("server gracefully stopped")
}
