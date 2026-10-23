package http

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/goopsagent/goopsagent/internal/agent"
	"github.com/goopsagent/goopsagent/internal/models"
	"github.com/goopsagent/goopsagent/internal/store"
)

// 流式任务端点
func handleStreamTask(orch *agent.Orchestrator, st *store.Store, w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message   string `json:"message"`
		SessionID string `json:"session_id"` // 可选，为空则自动创建
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}

	// 如果没有传入 session，创建一个临时 session
	sessionID := req.SessionID
	if sessionID == "" {
		sess, err := st.CreateSession("default", req.Message)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("create session: %v", err)})
			return
		}
		sessionID = sess.ID
	}

	// 创建任务记录
	task, err := st.CreateTask(sessionID, req.Message)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("create task: %v", err)})
		return
	}

	// 设置 SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // 禁用 Nginx 缓冲

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	// 发送初始事件（含 task_id 和 session_id，前端用于展示和后续查询）
	initData, _ := json.Marshal(map[string]string{
		"task_id":    task.ID,
		"session_id": sessionID,
	})
	fmt.Fprintf(w, "event: init\ndata: %s\n\n", string(initData))
	flusher.Flush()

	// 创建事件通道（带缓冲，防止阻塞）
	events := make(chan models.SSEEvent, 64)

	// 启动编排器
	go func() {
		defer close(events)

		result, err := orch.RunStreaming(r.Context(), req.Message, events)
		if err != nil {
			slog.Error("streaming task failed", "task_id", task.ID, "error", err)
			st.FailTask(task.ID, err.Error())
		} else {
			slog.Info("streaming task completed", "task_id", task.ID, "tool_calls", len(result.ToolRuns))

			// 将 tool_runs 转换为 models.ToolRun
			var modelToolRuns []models.ToolRun
			for _, tr := range result.ToolRuns {
				modelToolRuns = append(modelToolRuns, models.ToolRun{
					ToolName: tr.ToolName,
					Input:    json.RawMessage(tr.Input),
					Duration: tr.Result.DurationMs,
				})
			}
			st.CompleteTask(task.ID, result.Answer, modelToolRuns)
		}
	}()

	// 循环写入 SSE 事件
	for event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			slog.Error("failed to marshal sse event", "error", err)
			continue
		}

		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, string(data))

		flusher.Flush()

		// 发送 done 事件后跳出
		if event.Type == string(models.SSETypeDone) || event.Type == string(models.SSETypeError) {
			break
		}
	}
}
