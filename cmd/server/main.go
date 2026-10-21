package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/goopsagent/goopsagent/internal/agent"
	"github.com/goopsagent/goopsagent/internal/agent/provider"
	"github.com/goopsagent/goopsagent/internal/config"
	"github.com/goopsagent/goopsagent/internal/tools"
	httpserver "github.com/goopsagent/goopsagent/internal/transport/http"
)

func main() {
	//加载配置
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// 初始化日志
	setupLogging(cfg.Log)

	slog.Info("starting goopsagent",
		"port", cfg.Server.Port,
		"log_level", cfg.Log.Level,
	)

	// 创建 LLM 客户端
	llmClient := provider.NewClient(provider.Config{
		APIKey:    cfg.LLM.APIKey,
		BaseURL:   cfg.LLM.BaseURL,
		Model:     cfg.LLM.Model,
		Timeout:   cfg.LLM.Timeout,
		MaxTokens: cfg.LLM.MaxTokens,
	})

	// 注入到 HTTP 层（为聊天接口使用）
	httpserver.InitChatClientFromClient(llmClient)

	// 初始化工具注册中心
	registry := tools.NewRegistry()
	tools.RegisterDemoTools(registry)

	// 创建编排器
	orch := agent.NewOrchestrator(llmClient, registry, agent.Policy{
		MaxIterations: cfg.Agent.MaxIterations,
		MaxToolCalls:  cfg.Agent.MaxToolCalls,
		TaskTimeout:   cfg.Agent.TaskTimeout,
		ToolTimeout:   cfg.Agent.ToolTimeout,
	})

	// 创建 HTTP 服务
	server := httpserver.NewServer(
		cfg.Server.Port,
		cfg.Server.ReadTimeout,
		cfg.Server.WriteTimeout,
		orch,
		registry,
	)

	//监听退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-quit
	slog.Info("received shutdown signal")

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	slog.Info("server stopped")
}

func setupLogging(cfg config.LogConfig) {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if cfg.Format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))
}
