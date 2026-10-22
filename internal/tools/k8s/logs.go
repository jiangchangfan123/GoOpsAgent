package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/goopsagent/goopsagent/internal/tools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const maxLogBytes = 64 * 1024 // 单次最多返回 64KB 日志

// ---- get_pod_logs：获取 Pod 日志 ----

type GetPodLogsTool struct {
	client *Client
}

func (t *GetPodLogsTool) Name() string { return "get_pod_logs" }
func (t *GetPodLogsTool) Description() string {
	return "获取指定 Pod 中某个容器的日志，最多返回 64KB。用于诊断应用问题"
}

func (t *GetPodLogsTool) InputSchema() json.RawMessage {
	schema := tools.ParameterSchema{
		Type:        "object",
		Description: t.Description(),
		Required:    []string{"namespace", "name"},
		Properties: map[string]tools.ParamProp{
			"namespace":  {Type: "string", Description: "Kubernetes Namespace 名称"},
			"name":       {Type: "string", Description: "Pod 名称"},
			"container":  {Type: "string", Description: "容器名称（Pod 只有一个容器时可省略）"},
			"tail_lines": {Type: "integer", Description: "返回最后 N 行日志，默认 100"},
			"previous":   {Type: "boolean", Description: "是否获取上一个（已崩溃）容器的日志"},
		},
	}
	b, _ := json.Marshal(schema)
	return b
}

func (t *GetPodLogsTool) Execute(ctx context.Context, input json.RawMessage) (tools.ToolResult, error) {
	var params struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		Container string `json:"container"`
		TailLines int64  `json:"tail_lines"`
		Previous  bool   `json:"previous"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return tools.ErrorResult("invalid_input", "参数解析失败", 0), err
	}
	if params.Name == "" {
		return tools.ErrorResult("invalid_input", "pod name 是必填项", 0), fmt.Errorf("pod name is required")
	}
	if params.TailLines <= 0 {
		params.TailLines = 100
	}

	if err := t.client.CheckNamespace(params.Namespace); err != nil {
		return tools.ErrorResult("namespace_denied", err.Error(), 0), err
	}

	// 如果用户没有指定容器，自动发现
	//如果该pod只有一个container，则返回该container的日志，否则把所有容器名称作为结果，返回参数应该从里面选的error
	if params.Container == "" {
		pod, err := t.client.Clientset.CoreV1().Pods(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{})
		if err != nil {
			return tools.ErrorResult("k8s_api_error", fmt.Sprintf("获取 Pod 信息失败: %v", err), 0), err
		}
		if len(pod.Spec.Containers) == 1 {
			params.Container = pod.Spec.Containers[0].Name
		} else {
			names := make([]string, 0, len(pod.Spec.Containers))
			for _, c := range pod.Spec.Containers {
				names = append(names, c.Name)
			}
			return tools.ErrorResult("ambiguous_container",
				fmt.Sprintf("Pod 有 %d 个容器，请指定 container 参数：%v", len(names), names), 0), nil
		}
	}

	ctx, cancel := t.client.TimeoutContext(ctx)
	defer cancel()

	logOpts := &corev1.PodLogOptions{
		Container:  params.Container,
		TailLines:  &params.TailLines,
		Previous:   params.Previous,
		LimitBytes: &[]int64{maxLogBytes}[0],
	}

	req := t.client.Clientset.CoreV1().Pods(params.Namespace).GetLogs(params.Name, logOpts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return tools.ErrorResult("k8s_api_error", fmt.Sprintf("获取日志失败: %v", err), 0), err
	}
	defer stream.Close()

	logBytes, err := io.ReadAll(stream)
	if err != nil {
		return tools.ErrorResult("k8s_api_error", fmt.Sprintf("读取日志流失败: %v", err), 0), err
	}

	truncated := len(logBytes) >= maxLogBytes

	return tools.SuccessResult(map[string]any{
		"pod":       params.Name,
		"namespace": params.Namespace,
		"container": params.Container,
		"truncated": truncated,
		"log":       string(logBytes),
	}, "kubernetes", 0), nil
}

// RegisterLogTools 注册日志工具
func RegisterLogTools(registry *tools.Registry, client *Client) {
	registry.Register(&GetPodLogsTool{client: client})
}
