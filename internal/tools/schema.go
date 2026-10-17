package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type ToolResult struct {
	Status     string `json:"status"`
	Data       any    `json:"data"`
	ErrorCode  string `json:"error_code,omitempty"`
	Message    string `json:"message,omitempty"`
	Truncated  bool   `json:"truncated"`
	DurationMs int64  `json:"duration_ms"`
	Source     string `json:"source"` // k8s / rag / internal
}

type Tool interface {
	Name() string
	Description() string
	InputSchema() json.RawMessage
	Execute(ctx context.Context, input json.RawMessage) (ToolResult, error)
}

type ParameterSchema struct {
	Type        string               `json:"type"`
	Description string               `json:"description"`
	Required    []string             `json:"required,omitempty"` //必选字段名单
	Properties  map[string]ParamProp `json:"properties"`         //每个字段是什么样的详细定义
}

type ParamProp struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

func ValidateInput(schema json.RawMessage, input json.RawMessage) error {
	var ps ParameterSchema
	if err := json.Unmarshal(schema, &ps); err != nil {
		return fmt.Errorf("invalid schema: %w", err)
	}

	var inputMap map[string]any
	if err := json.Unmarshal(input, &inputMap); err != nil {
		return fmt.Errorf("invalid input json: %w", err)
	}

	//检查有没有缺失的必选字段
	for _, field := range ps.Required {
		if _, ok := inputMap[field]; !ok {
			return fmt.Errorf("required field %s is missing", field)
		}
	}

	//检查输入字段是否符合schema定义的类型
	for key := range inputMap {
		prop, ok := ps.Properties[key]
		if !ok {
			return fmt.Errorf("unknown field: %s", key)
		}
		if err := checkType(prop.Type, inputMap[key]); err != nil {
			return fmt.Errorf("field %s: %w", key, err)
		}
	}

	return nil
}

func checkType(expected string, val any) error {
	switch expected {
	case "string":
		if _, ok := val.(string); !ok {
			return fmt.Errorf("expected string, got %T", val)
		}
	case "number":
		switch val.(type) {
		case float64, int, int64, json.Number:
		default:
			return fmt.Errorf("expected number, got %T", val)
		}
	}
	return nil
}

// SuccessResult 快捷构造成功结果
func SuccessResult(data any, source string, duration time.Duration) ToolResult {
	return ToolResult{
		Status:     "success",
		Data:       data,
		Source:     source,
		DurationMs: duration.Milliseconds(),
	}
}

// ErrorResult 快捷构造错误结果
func ErrorResult(errorCode, message string, duration time.Duration) ToolResult {
	return ToolResult{
		Status:     "error",
		ErrorCode:  errorCode,
		Message:    message,
		DurationMs: duration.Milliseconds(),
	}
}
