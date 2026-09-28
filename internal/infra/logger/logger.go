package logger

import (
	"fmt"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/infra/config"
	"go.uber.org/zap"
)

func NewLogger(cfg config.Config) (*zap.Logger, error) {
	if cfg.AppEnv == "production" {
		logger, err := zap.NewProduction()
		if err != nil {
			return nil, fmt.Errorf("error creating production logger: %w", err)
		}
		return logger, nil
	}
	logger, err := zap.NewDevelopment()
	if err != nil {
		return nil, fmt.Errorf("error creating development logger: %w", err)
	}
	return logger, nil
}
