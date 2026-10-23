package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/goopsagent/goopsagent/internal/agent/provider"
	"github.com/goopsagent/goopsagent/internal/models"
	"github.com/goopsagent/goopsagent/internal/tools"
)

type Orchestrator struct {
	llm      *provider.Client
	registry *tools.Registry
	policy   Policy
}

// ToolCall 模型发出的工具调用请求
type ToolCall struct {
	Name  string          `json:"tool"`
	Input json.RawMessage `json:"input"`
}

// ToolRunRecord 单次工具调用记录
type ToolRunRecord struct {
	ToolName string           `json:"tool_name"`
	Input    string           `json:"input"`
	Result   tools.ToolResult `json:"result"`
}

// RunResult Agent 执行结果
type RunResult struct {
	Answer   string          `json:"answer"`
	ToolRuns []ToolRunRecord `json:"tool_runs,omitempty"`
}

// NewOrchestrator 创建编排器
func NewOrchestrator(llm *provider.Client, registry *tools.Registry, policy Policy) *Orchestrator {
	return &Orchestrator{
		llm:      llm,
		registry: registry,
		policy:   policy,
	}
}

type RunResponse = RunResult

// Run 执行 Agent 任务，返回最终回复和工具调用记录
func (o *Orchestrator) Run(ctx context.Context, userMessage string) (*RunResult, error) {
	tracker := NewExecutionTracker(o.policy)

	messages := []provider.ChatMessage{
		{
			Role:    "system",
			Content: BuildSystemPrompt(o.registry.ToToolDefs()),
		},
		{
			Role:    "user",
			Content: userMessage,
		},
	}

	var toolRuns []ToolRunRecord

	for {
		//检查整体超时
		if err := tracker.CheckTimeout(); err != nil {
			return o.finalResult(messages, toolRuns, fmt.Errorf("task timeout: %w", err))
		}

		// 检查迭代次数
		if err := tracker.IncIterations(); err != nil {
			return o.finalResult(messages, toolRuns, fmt.Errorf("max iterations: %w", err))
		}

		slog.Info("agent iteration",
			"iteration", tracker.iterations,
			"messages_count", len(messages),
		)

		//调用模型，支持工具调用
		resp, err := o.llm.Chat(ctx, messages)
		if err != nil {
			return o.finalResult(messages, toolRuns, fmt.Errorf("llm call: %w", err))
		}

		if len(resp.Choices) == 0 {
			return o.finalResult(messages, toolRuns, fmt.Errorf("no choices in response"))
		}

		choice := resp.Choices[0]
		assistantMsg := choice.Message

		//将模型回复加入历史消息
		messages = append(messages, assistantMsg)

		//检查是否有工具调用
		toolCall := o.parseToolCall(assistantMsg.Content)
		if toolCall == nil {
			// 没有工具调用，模型给出了最终回复
			slog.Info("agent finished - no tool call")
			return &RunResult{
				Answer:   assistantMsg.Content,
				ToolRuns: toolRuns,
			}, nil
		}

		// 检查工具调用次数
		if err := tracker.IncToolCall(); err != nil {
			return o.finalResult(messages, toolRuns, fmt.Errorf("max tool calls: %w", err))
		}

		// 执行工具
		toolRun := o.executeTool(ctx, tracker, toolCall.Name, toolCall.Input)
		toolRuns = append(toolRuns, toolRun)

		// 将工具结果加入消息历史
		resultJSON, _ := json.Marshal(toolRun.Result)
		messages = append(messages, provider.ChatMessage{
			Role:    "user",
			Content: fmt.Sprintf("工具 %s 的执行结果：\n%s", toolCall.Name, string(resultJSON)),
		})
	}
}

// parseToolCall 从模型回复中解析工具调用（JSON 格式）
func (o *Orchestrator) parseToolCall(content string) *ToolCall {
	// 查找 JSON 块中的工具调用
	// 模型可能返回类似: {"tool": "list_pods", "input": {...}} 的格式
	clean := extractJSON(content)
	if clean == "" {
		return nil
	}

	var tc ToolCall
	if err := json.Unmarshal([]byte(clean), &tc); err != nil {
		return nil
	}
	if tc.Name == "" {
		return nil
	}
	return &tc
}

// extractJSON 从文本中提取第一个 JSON 对象
func extractJSON(content string) string {
	start := -1
	depth := 0
	for i, ch := range content {
		if ch == '{' {
			if start == -1 {
				start = i
			}
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 && start != -1 {
				return content[start : i+1]
			}
		}
	}
	return ""
}

// 执行单个工具
func (o *Orchestrator) executeTool(ctx context.Context, tracker *ExecutionTracker, name string, input json.RawMessage) ToolRunRecord {
	record := ToolRunRecord{
		ToolName: name,
		Input:    string(input),
	}

	tool, ok := o.registry.Get(name)
	if !ok {
		record.Result = tools.ErrorResult("unknown_tool",
			fmt.Sprintf("tool %s not registered", name), 0)
		return record
	}

	// 校验输入
	if err := tools.ValidateInput(tool.InputSchema(), input); err != nil {
		record.Result = tools.ErrorResult("invalid_input", err.Error(), 0)
		return record
	}

	// 带超时的上下文
	toolCtx, cancel := tracker.ToolContext(ctx)
	defer cancel()

	// 执行
	slog.Info("executing tool", "tool", name)
	result, err := tool.Execute(toolCtx, input)
	if err != nil {
		record.Result = tools.ErrorResult("execution_error", err.Error(), 0)
		return record
	}
	record.Result = result
	return record
}

// finalResult 构造最终结果（异常终止时）
func (o *Orchestrator) finalResult(messages []provider.ChatMessage, toolRuns []ToolRunRecord, err error) (*RunResult, error) {
	if len(messages) > 0 {
		last := messages[len(messages)-1]
		if last.Role == "assistant" && last.Content != "" {
			return &RunResult{
				Answer:   fmt.Sprintf("%s\n\n[任务异常终止: %v]", last.Content, err),
				ToolRuns: toolRuns,
			}, nil
		}
	}
	return nil, err
}

// RunStreaming流式执行agent任务，通过channel推送sse事件
func (o *Orchestrator) RunStreaming(ctx context.Context, userMessage string, events chan<- models.SSEEvent) (*RunResponse, error) {
	tracker := NewExecutionTracker(o.policy)

	//发送开始事件
	events <- models.SSEEvent{
		Type: string(models.SSETypeStatus),
		Data: map[string]string{"status": "running"},
	}

	messages := []provider.ChatMessage{
		{Role: "system", Content: BuildSystemPrompt(o.registry.ToToolDefs())},
		{Role: "user", Content: userMessage},
	}

	var toolRuns []ToolRunRecord

	for {
		//检查整体超时
		if err := tracker.CheckTimeout(); err != nil {
			events <- models.SSEEvent{
				Type: string(models.SSETypeError),
				Data: map[string]string{"message": fmt.Sprintf("task timeout: %v", err)},
			}
			return o.streamingFinalResult(messages, toolRuns, events, fmt.Errorf("task timeout: %w", err))
		}

		//检查工具调用次数
		if err := tracker.IncIterations(); err != nil {
			events <- models.SSEEvent{
				Type: string(models.SSETypeError),
				Data: map[string]string{"message": fmt.Sprintf("max iterations reached: %v", err)},
			}
			return o.streamingFinalResult(messages, toolRuns, events, fmt.Errorf("max iterations: %w", err))
		}

		slog.Info("agent iteration (streaming)",
			"iteration", tracker.iterations,
			"messages_count", len(messages),
		)

		// 调用模型
		resp, err := o.llm.Chat(ctx, messages)
		if err != nil {
			events <- models.SSEEvent{
				Type: string(models.SSETypeError),
				Data: map[string]string{"message": fmt.Sprintf("llm call failed: %v", err)},
			}
			return o.streamingFinalResult(messages, toolRuns, events, fmt.Errorf("llm call: %w", err))
		}

		if len(resp.Choices) == 0 {
			events <- models.SSEEvent{
				Type: string(models.SSETypeError),
				Data: map[string]string{"message": "no choices in llm response"},
			}
			return o.streamingFinalResult(messages, toolRuns, events, fmt.Errorf("no choices in response"))
		}

		choice := resp.Choices[0]
		assistantMsg := choice.Message
		messages = append(messages, assistantMsg) // 添加到消息列表

		// 检查是否有工具调用
		toolCall := o.parseToolCall(assistantMsg.Content)
		if toolCall == nil {
			//如果没有工具调用，则大模型给出了最终回复
			slog.Info("agent finished (streaming) - no tool call")

			//发送最终内容
			events <- models.SSEEvent{
				Type: string(models.SSETypeContent),
				Data: map[string]string{"content": assistantMsg.Content},
			}

			// 发送完成事件
			events <- models.SSEEvent{
				Type: string(models.SSETypeDone),
				Data: map[string]any{
					"answer":    assistantMsg.Content,
					"tool_runs": toolRuns,
				},
			}
			return &RunResponse{
				Answer:   assistantMsg.Content,
				ToolRuns: toolRuns,
			}, nil
		}

		// 检查工具调用次数
		if err := tracker.IncToolCall(); err != nil {
			events <- models.SSEEvent{
				Type: string(models.SSETypeError),
				Data: map[string]string{"message": fmt.Sprintf("max tool calls reached: %v", err)},
			}
			return o.streamingFinalResult(messages, toolRuns, events, fmt.Errorf("max tool calls: %w", err))
		}

		// 发送工具调用开始事件
		events <- models.SSEEvent{
			Type: string(models.SSETypeToolCall),
			Data: map[string]string{
				"tool":  toolCall.Name,
				"input": string(toolCall.Input),
			},
		}

		// 执行工具
		toolRun := o.executeTool(ctx, tracker, toolCall.Name, toolCall.Input)
		toolRuns = append(toolRuns, toolRun) // 添加到工具调用记录

		// 发送工具结果事件
		events <- models.SSEEvent{
			Type: string(models.SSETypeToolResult),
			Data: map[string]any{
				"tool":     toolRun.ToolName,
				"status":   toolRun.Result.Status,
				"data":     toolRun.Result.Data,
				"duration": toolRun.Result.DurationMs,
				"error":    toolRun.Result.ErrorCode,
			},
		}

		// 将工具结果加入消息历史
		resultJSON, _ := json.Marshal(toolRun.Result)
		messages = append(messages, provider.ChatMessage{
			Role:    "user",
			Content: fmt.Sprintf("工具 %s 的执行结果：\n%s", toolCall.Name, string(resultJSON)),
		})
	}
}

// 流式模式下的异常终止
func (o *Orchestrator) streamingFinalResult(messages []provider.ChatMessage, toolRuns []ToolRunRecord, events chan<- models.SSEEvent, err error) (*RunResponse, error) {
	if len(messages) > 0 {
		last := messages[len(messages)-1]
		if last.Role == "assistant" && last.Content != "" {
			ans := fmt.Sprintf("%s\n\n[任务异常终止: %v]", last.Content, err)
			events <- models.SSEEvent{
				Type: string(models.SSETypeDone),
				Data: map[string]any{
					"answer":    ans,
					"tool_runs": toolRuns,
				},
			}
			return &RunResponse{Answer: ans, ToolRuns: toolRuns}, nil
		}
	}
	events <- models.SSEEvent{
		Type: string(models.SSETypeError),
		Data: map[string]string{"message": err.Error()},
	}
	return nil, err
}
