package models

import (
	"encoding/json"
	"time"
)

type TaskStatus string

const (
	TaskStatusCreated      TaskStatus = "created"       // 已创建，等待执行
	TaskStatusRunning      TaskStatus = "running"       // Agent 正在思考/调用工具
	TaskStatusWaitingInput TaskStatus = "waiting_input" // 等待用户补充信息（预留）
	TaskStatusSucceeded    TaskStatus = "succeeded"     // 成功完成
	TaskStatusFailed       TaskStatus = "failed"        // 执行失败
	TaskStatusCancelled    TaskStatus = "cancelled"     // 用户取消
)

type Task struct {
	ID        string     `json:"id"`
	SessionID string     `json:"session_id"`
	Message   string     `json:"message"` // 用户原始提问
	Status    TaskStatus `json:"status"`
	Answer    string     `json:"answer"`    // Agent 最终回复
	ToolRuns  []ToolRun  `json:"tool_runs"` // 工具调用记录（JSON 列）
	Error     string     `json:"error"`     // 失败时的错误信息
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// ToolRun 记录单次工具调用
type ToolRun struct {
	ToolName string          `json:"tool_name"`
	Input    json.RawMessage `json:"input"`
	Output   json.RawMessage `json:"output"`
	Duration int64           `json:"duration_ms"`
	Error    string          `json:"error,omitempty"`
}

// Session 是一次用户对话
type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"` // active / archived
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Message 是对话中的一条消息
type Message struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	TaskID    string    `json:"task_id,omitempty"`
	Role      string    `json:"role"` // user / assistant / system / tool
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// SSEEvent SSE 推送的事件结构
type SSEEvent struct {
	Type string `json:"type"` // status / content / tool_call / tool_result / error / done
	Data any    `json:"data"`
}

// SSE 事件类型常量
const (
	SSETypeStatus     = "status"
	SSETypeContent    = "content"
	SSETypeToolCall   = "tool_call"
	SSETypeToolResult = "tool_result"
	SSETypeError      = "error"
	SSETypeDone       = "done"
)
