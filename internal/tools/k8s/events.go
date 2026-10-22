package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/goopsagent/goopsagent/internal/tools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//在 Kubernetes 中，Event（事件）是集群内状态变化的报告或记录，主要用于解释“某个对象为什么处于当前状态”。它本身不是长期审计日志，而是一种辅助性的、尽力而为的观测数据，保留时间通常有限。

// ---- list_events：列出 Namespace 下的 Event ----

type ListEventsTool struct {
	client *Client
}

func (t *ListEventsTool) Name() string { return "list_events" }
func (t *ListEventsTool) Description() string {
	return "列出 Namespace 下的 Kubernetes Event，用于诊断异常，默认返回最近 50 条"
}

func (t *ListEventsTool) InputSchema() json.RawMessage {
	schema := tools.ParameterSchema{
		Type:        "object",
		Description: t.Description(),
		Required:    []string{"namespace"},
		Properties: map[string]tools.ParamProp{
			"namespace": {Type: "string", Description: "Kubernetes Namespace 名称"},
			"limit":     {Type: "integer", Description: "最多返回条数，默认 50"},
		},
	}
	b, _ := json.Marshal(schema)
	return b
}

func (t *ListEventsTool) Execute(ctx context.Context, input json.RawMessage) (tools.ToolResult, error) {
	var params struct {
		Namespace string `json:"namespace"`
		Limit     int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return tools.ErrorResult("invalid_input", "参数解析失败", 0), err
	}
	//默认限制为50条
	if params.Limit <= 0 {
		params.Limit = 50
	}

	if err := t.client.CheckNamespace(params.Namespace); err != nil {
		return tools.ErrorResult("namespace_denied", err.Error(), 0), err
	}

	ctx, cancel := t.client.TimeoutContext(ctx)
	defer cancel()

	events, err := t.client.Clientset.CoreV1().Events(params.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return tools.ErrorResult("k8s_api_error", fmt.Sprintf("查询 Event 列表失败: %v", err), 0), err
	}

	// 按时间倒序：最新的 Event 排最前面
	sort.Slice(events.Items, func(i, j int) bool {
		return events.Items[i].LastTimestamp.After(events.Items[j].LastTimestamp.Time)
	})

	type eventInfo struct {
		Type     string `json:"type"`
		Reason   string `json:"reason"`
		Message  string `json:"message"`
		Involved string `json:"involved"`
		Count    int32  `json:"count"`
		LastSeen string `json:"last_seen"`
	}
	result := make([]eventInfo, 0, min(len(events.Items), params.Limit))
	for _, e := range events.Items {
		if len(result) >= params.Limit {
			break
		}
		involved := ""
		if e.InvolvedObject.Kind != "" && e.InvolvedObject.Name != "" {
			involved = fmt.Sprintf("%s/%s", e.InvolvedObject.Kind, e.InvolvedObject.Name)
		}
		result = append(result, eventInfo{
			Type:     e.Type,
			Reason:   e.Reason,
			Message:  e.Message,
			Involved: involved,
			Count:    e.Count,
			LastSeen: e.LastTimestamp.Format("2006-01-02T15:04:05Z"),
		})
	}

	return tools.SuccessResult(map[string]any{
		"count":  len(result),
		"events": result,
	}, "kubernetes", 0), nil
}

// RegisterEventTools 注册 Event 工具
func RegisterEventTools(registry *tools.Registry, client *Client) {
	registry.Register(&ListEventsTool{client: client})
}
