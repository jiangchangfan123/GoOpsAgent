package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/goopsagent/goopsagent/internal/agent"
	"github.com/goopsagent/goopsagent/internal/agent/provider"
	"github.com/goopsagent/goopsagent/internal/config"
	"github.com/goopsagent/goopsagent/internal/knowledge"
	"github.com/goopsagent/goopsagent/internal/store"
	"github.com/goopsagent/goopsagent/internal/tools"
	k8stools "github.com/goopsagent/goopsagent/internal/tools/k8s"
	httpserver "github.com/goopsagent/goopsagent/internal/transport/http"
	goredis "github.com/redis/go-redis/v9"
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

	// ---- 初始化数据层 ----
	var (
		db  *sql.DB
		rdb *goredis.Client
		st  *store.Store
	)

	// 尝试连接 MySQL（可选，DSN 为空则跳过）
	if cfg.Database.DSN != "" {
		var err error
		db, err = store.NewDB(
			cfg.Database.DSN,
			cfg.Database.MaxOpenConns,
			cfg.Database.MaxIdleConns,
			cfg.Database.ConnMaxLifetime,
		)
		if err != nil {
			slog.Warn("mysql unavailable, running without persistence", "error", err)
			db = nil
		} else {
			defer db.Close()
			slog.Info("mysql ready")
		}
	}

	// 尝试连接 Redis（可选，地址为空则跳过）
	if cfg.Redis.Addr != "" {
		var err error
		rdb, err = store.NewRedis(cfg.Redis.Addr, cfg.Redis.Password)
		if err != nil {
			slog.Warn("redis unavailable", "error", err)
			rdb = nil
		}
	}

	// 创建统一数据层（DB 或 Redis 任一可用即创建；都为空时 st 为 nil）
	if db != nil || rdb != nil {
		st = store.New(db, rdb)
	}

	// ---- 初始化工具注册中心 ----
	registry := tools.NewRegistry()
	tools.RegisterDemoTools(registry)

	// 初始化 K8s 客户端（可选，K8s 模式为空时跳过）
	if cfg.Kubernetes.Mode != "" {
		k8sClient, err := k8stools.NewClient(k8stools.ClientConfig{
			Mode:               cfg.Kubernetes.Mode,
			KubeconfigPath:     cfg.Kubernetes.KubeconfigPath,
			Timeout:            cfg.Kubernetes.DefaultTimeout,
			NamespaceWhitelist: cfg.Kubernetes.NamespaceWhitelist,
		})
		if err != nil {
			slog.Warn("failed to create k8s client, k8s tools disabled", "error", err)
		} else {
			k8stools.RegisterPodTools(registry, k8sClient)
			k8stools.RegisterDeploymentTools(registry, k8sClient)
			k8stools.RegisterEventTools(registry, k8sClient)
			k8stools.RegisterLogTools(registry, k8sClient)
			slog.Info("k8s tools registered")
		}
	}

	// 创建编排器
	orch := agent.NewOrchestrator(llmClient, registry, agent.Policy{
		MaxIterations: cfg.Agent.MaxIterations,
		MaxToolCalls:  cfg.Agent.MaxToolCalls,
		TaskTimeout:   cfg.Agent.TaskTimeout,
		ToolTimeout:   cfg.Agent.ToolTimeout,
	})

	// ---- 初始化 RAG 知识检索器 ----
	// 尝试加载索引文件；不存在时跳过，Agent 不需要 RAG
	indexPath := os.Getenv("GOOPSAGENT_KNOWLEDGE_INDEX")
	if indexPath == "" {
		indexPath = "internal/knowledge/index.json"
	}
	if data, err := os.ReadFile(indexPath); err == nil {
		slog.Info("loading knowledge index", "path", indexPath)
		var index struct {
			Chunks []knowledge.Chunk `json:"chunks"`
		}
		if err := json.Unmarshal(data, &index); err == nil && len(index.Chunks) > 0 {
			// 训练 Embedder
			emb := knowledge.NewEmbedder()
			corpus := make([]string, len(index.Chunks))
			for i, c := range index.Chunks {
				corpus[i] = c.Content
			}
			emb.Fit(corpus)

			// 创建检索器
			retriever := knowledge.NewRetriever(emb, knowledge.RetrieverConfig{
				MaxResults: 3,
				MinScore:   0.2,
			})
			retriever.Index(index.Chunks)

			// 注入 Orchestrator
			orch.SetRetriever(retriever)
			slog.Info("rag retriever ready", "chunks", len(index.Chunks), "vocab", emb.Dim())
		} else {
			slog.Info("knowledge index empty, skipping RAG", "path", indexPath)
		}
	} else {
		slog.Info("knowledge index not found, skipping RAG",
			"path", indexPath,
			"tip", "run: go run cmd/ingest/main.go knowledge/ internal/knowledge/index.json",
		)
	}

	// 创建 HTTP 服务
	server := httpserver.NewServer(
		cfg.Server.Port,
		cfg.Server.ReadTimeout,
		cfg.Server.WriteTimeout,
		orch,
		registry,
		st,
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
