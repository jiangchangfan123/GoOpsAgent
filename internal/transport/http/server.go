package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/goopsagent/goopsagent/internal/middleware"
)

type Server struct {
	httpServer *http.Server
	port       int
}

// 创建http服务
func NewServer(port int, readTimeout, writeTimeout time.Duration) *Server {
	mux := http.NewServeMux()

	// 注册路由
	mux.HandleFunc("GET /api/v1/health/live", handleLive)
	mux.HandleFunc("GET /api/v1/health/ready", handleReady)

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

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
