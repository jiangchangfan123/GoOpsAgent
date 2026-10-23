package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/goopsagent/goopsagent/internal/agent"
	"github.com/goopsagent/goopsagent/internal/agent/provider"
	"github.com/goopsagent/goopsagent/internal/diagnostics"
	"github.com/goopsagent/goopsagent/internal/middleware"
	"github.com/goopsagent/goopsagent/internal/models"
	"github.com/goopsagent/goopsagent/internal/store"
	"github.com/goopsagent/goopsagent/internal/tools"
)

type Server struct {
	httpServer   *http.Server
	port         int
	orchestrator *agent.Orchestrator
	registry     *tools.Registry
	store        *store.Store
}

// chatClient 全局 LLM 客户端（阶段 1 简单用全局变量，后续阶段重构为依赖注入）
var chatClient *provider.Client

// InitChatClientFromClient 使用已创建的 LLM 客户端初始化
func InitChatClientFromClient(client *provider.Client) {
	chatClient = client
}

// 创建http服务
func NewServer(port int, readTimeout, writeTimeout time.Duration, orch *agent.Orchestrator, reg *tools.Registry, st *store.Store) *Server {
	mux := http.NewServeMux()

	// 健康检查
	mux.HandleFunc("GET /api/v1/health/live", handleLive)
	mux.HandleFunc("GET /api/v1/health/ready", handleReady)

	// 聊天和 Agent 任务
	mux.HandleFunc("POST /api/v1/chat", handleChat)
	mux.HandleFunc("POST /api/v1/tasks", func(w http.ResponseWriter, r *http.Request) {
		handleCreateTask(orch, w, r)
	})
	mux.HandleFunc("POST /api/v1/stream", func(w http.ResponseWriter, r *http.Request) {
		handleStreamTask(orch, st, w, r)
	})

	// 任务查询与诊断报告（store 可能为 nil，通过闭包透传）
	mux.HandleFunc("GET /api/v1/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		handleGetTask(st, w, r)
	})
	mux.HandleFunc("GET /api/v1/tasks/{id}/report", func(w http.ResponseWriter, r *http.Request) {
		handleGetReport(st, w, r)
	})

	//中间件链
	var handler http.Handler = mux
	handler = middleware.RequestID(handler)
	handler = middleware.Logging(handler)
	handler = middleware.Recovery(handler)
	handler = middleware.Auth(handler)

	return &Server{
		port:         port,
		orchestrator: orch,
		registry:     reg,
		store:        st,
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

// 创建并执行agent任务
func handleCreateTask(orch *agent.Orchestrator, w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message string `json:"message"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}

	result, err := orch.Run(r.Context(), req.Message)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

// handleGetTask 按 ID 查询任务
func handleGetTask(st *store.Store, w http.ResponseWriter, r *http.Request) {
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store not available"})
		return
	}

	taskID := r.PathValue("id")
	if taskID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task id is required"})
		return
	}

	task, err := st.GetTask(taskID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, task)
}

// handleGetReport 生成诊断报告
func handleGetReport(st *store.Store, w http.ResponseWriter, r *http.Request) {
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store not available"})
		return
	}

	taskID := r.PathValue("id")
	if taskID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task id is required"})
		return
	}

	task, err := st.GetTask(taskID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	// 任务尚未完成时不能生成报告
	if task.Status != models.TaskStatusSucceeded {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error":  "task not completed yet",
			"status": string(task.Status),
		})
		return
	}

	report := diagnostics.BuildReport(task)
	writeJSON(w, http.StatusOK, report)
}
