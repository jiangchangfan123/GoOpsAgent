package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/goopsagent/goopsagent/internal/agent/provider"
	"github.com/goopsagent/goopsagent/internal/config"
	"github.com/goopsagent/goopsagent/internal/middleware"
)

type Server struct {
	httpServer *http.Server
	port       int
}

// chatClient 全局 LLM 客户端（阶段 1 简单用全局变量，后续阶段重构为依赖注入）
var chatClient *provider.Client

// InitChatClient 初始化 LLM 客户端，在 main.go 中调用
func InitChatClient(cfg config.LLMConfig) {
	chatClient = provider.NewClient(provider.Config{
		APIKey:    cfg.APIKey,
		BaseURL:   cfg.BaseURL,
		Model:     cfg.Model,
		Timeout:   cfg.Timeout,
		MaxTokens: cfg.MaxTokens,
	})
}

// 创建http服务
func NewServer(port int, readTimeout, writeTimeout time.Duration) *Server {
	mux := http.NewServeMux()

	// 注册路由
	mux.HandleFunc("GET /api/v1/health/live", handleLive)
	mux.HandleFunc("GET /api/v1/health/ready", handleReady)
	mux.HandleFunc("POST /api/v1/chat", handleChat)

	//中间件链
	var handler http.Handler = mux
	handler = middleware.RequestID(handler)
	handler = middleware.Logging(handler)
	handler = middleware.Recovery(handler)
	handler = middleware.Auth(handler)

	return &Server{
		port: port,
		httpServer: &http.Server{
			Addr:         fmt.Sprintf(":%d", port),
			Handler:      handler,
			ReadTimeout:  readTimeout,
			WriteTimeout: writeTimeout,
		},
	}
}

// Start 启动服务（阻塞）
func (s *Server) Start() error {
	slog.Info("starting http server", "port", s.port)
	return s.httpServer.ListenAndServe()
}

// Shutdown 优雅退出
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("shutting down http server")
	return s.httpServer.Shutdown(ctx)
}

// ---- 健康检查处理器 ----

func handleLive(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
}

func handleReady(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	if chatClient == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "llm client not initialized"})
		return
	}

	var req struct {
		Messages []provider.ChatMessage `json:"messages"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if len(req.Messages) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "messages is required"})
		return
	}

	resp, err := chatClient.Chat(r.Context(), req.Messages)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if llmErr, ok := err.(*provider.LLMError); ok {
			if llmErr.StatusCode > 0 {
				statusCode = llmErr.StatusCode
			}
			if llmErr.Retryable && statusCode < 500 {
				statusCode = http.StatusTooManyRequests
			}
		}
		writeJSON(w, statusCode, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
