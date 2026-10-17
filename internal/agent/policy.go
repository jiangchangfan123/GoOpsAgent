package agent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Policy struct {
	MaxIterations int           //最大模型调用次数
	MaxToolCalls  int           //最大工具调用次数
	TaskTimeout   time.Duration //任务超时时间，单位秒
	ToolTimeout   time.Duration //单次工具超时时间，单位秒
}

// 跟踪执行过程中的限制
type ExecutionTracker struct {
	mu         sync.Mutex
	iterations int
	toolCalls  int
	policy     Policy
	startTime  time.Time
}

// NewExecutionTracker 创建跟踪器
func NewExecutionTracker(policy Policy) *ExecutionTracker {
	return &ExecutionTracker{
		policy:    policy,
		startTime: time.Now(),
	}
}

// 增加迭代次数，超过限制则返回error
func (t *ExecutionTracker) IncIterations() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.iterations++
	if t.iterations > t.policy.MaxIterations {
		return fmt.Errorf("max iterations (%d) exceeded", t.policy.MaxIterations)
	}
	return nil
}

// IncToolCall 增加工具调用计数，超过限制返回 error
func (t *ExecutionTracker) IncToolCall() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.toolCalls++
	if t.toolCalls > t.policy.MaxToolCalls {
		return fmt.Errorf("max tool calls (%d) exceeded", t.policy.MaxToolCalls)
	}
	return nil
}

// CheckTimeout 检查是否超过任务总超时
func (t *ExecutionTracker) CheckTimeout() error {
	if time.Since(t.startTime) > t.policy.TaskTimeout {
		return fmt.Errorf("task timeout (%s) exceeded", t.policy.TaskTimeout)
	}
	return nil
}

// 创建单次工具调用的 context，带超时
func (t *ExecutionTracker) ToolContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, t.policy.ToolTimeout)
}

// Stats 返回执行统计
func (t *ExecutionTracker) Stats() (iterations, toolCalls int, elapsed time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.iterations, t.toolCalls, time.Since(t.startTime)
}
