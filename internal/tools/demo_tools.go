package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"
)

// ---- 工具 1：echo - 回显输入 ----

type EchoTool struct{}

func (t *EchoTool) Name() string {
	return "echo"
}

func (t *EchoTool) Description() string {
	return "回显用户输入的内容，用于测试工具调用链路是否正常"
}

func (t *EchoTool) InputSchema() json.RawMessage {
	schema := ParameterSchema{
		Type:        "object",
		Description: t.Description(),
		Required:    []string{"message"},
		Properties: map[string]ParamProp{
			"message": {Type: "string", Description: "要回显的内容"},
		},
	}
	b, _ := json.Marshal(schema)
	return b
}

func (t *EchoTool) Execute(ctx context.Context, input json.RawMessage) (ToolResult, error) {
	var params struct {
		Message string `json:"message"`
	}
	json.Unmarshal(input, &params)

	return SuccessResult(map[string]string{
		"echo": params.Message,
	}, "internal", 0), nil
}

// ---- 工具 2：get_time - 获取当前时间 ----

type TimeTool struct{}

func (t *TimeTool) Name() string {
	return "get_time"
}

func (t *TimeTool) Description() string {
	return "获取当前服务器时间"
}

func (t *TimeTool) InputSchema() json.RawMessage {
	schema := ParameterSchema{
		Type:        "object",
		Description: t.Description(),
		Required:    []string{},
		Properties:  map[string]ParamProp{},
	}
	b, _ := json.Marshal(schema)
	return b
}

func (t *TimeTool) Execute(ctx context.Context, input json.RawMessage) (ToolResult, error) {
	now := time.Now()
	return SuccessResult(map[string]string{
		"time":     now.Format(time.RFC3339),
		"timezone": now.Location().String(),
	}, "internal", 0), nil
}

// RegisterDemoTools 将示范工具注册到注册中心
func RegisterDemoTools(registry *Registry) {
	registry.Register(&EchoTool{})
	registry.Register(&TimeTool{})
}

// ---- 工具 3：简单故障模拟（帮助测试错误处理） ----

type FaultSimTool struct{}

func (t *FaultSimTool) Name() string {
	return "fault_sim"
}

func (t *FaultSimTool) Description() string {
	return "模拟故障，用于测试 Agent 的错误处理能力"
}

func (t *FaultSimTool) InputSchema() json.RawMessage {
	schema := ParameterSchema{
		Type:        "object",
		Description: t.Description(),
		Required:    []string{"mode"},
		Properties: map[string]ParamProp{
			"mode": {Type: "string", Description: "故障模式: timeout, error, 或 success"},
		},
	}
	b, _ := json.Marshal(schema)
	return b
}

func (t *FaultSimTool) Execute(ctx context.Context, input json.RawMessage) (ToolResult, error) {
	var params struct {
		Mode string `json:"mode"`
	}
	json.Unmarshal(input, &params)

	switch params.Mode {
	case "timeout":
		select {
		case <-time.After(5 * time.Second):
			return SuccessResult(map[string]string{"status": "woke up"}, "internal", 5*time.Second), nil
		case <-ctx.Done():
			return ErrorResult("timeout", "模拟超时已触发", 0), ctx.Err()
		}
	case "error":
		return ErrorResult("fault_injected", "模拟故障已注入", 0), fmt.Errorf("simulated fault")
	default:
		return SuccessResult(map[string]string{"status": "ok"}, "internal", 0), nil
	}
}

func init() {
	_ = rand.Intn(1)
}