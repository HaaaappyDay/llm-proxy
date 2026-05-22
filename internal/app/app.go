package app

import (
	"github.com/lotus/llm-proxy/internal/auth"
	"github.com/lotus/llm-proxy/internal/config"
)

type App struct {
	Config  *config.Config
	Codex   *auth.CodexOAuthManager
	Copilot *auth.CopilotAuthManager
	APIKeys *auth.APIKeyManager
}

func New(cfg *config.Config) *App {
	return &App{
		Config:  cfg,
		Codex:   auth.NewCodexOAuthManager(cfg.DataDir),
		Copilot: auth.NewCopilotAuthManager(cfg.DataDir),
		APIKeys: auth.NewAPIKeyManager(cfg.DataDir),
	}
}
