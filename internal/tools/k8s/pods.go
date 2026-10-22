package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/goopsagent/goopsagent/internal/tools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ListPodsTool struct {
	client *Client
}

func (t *ListPodsTool) Name() string { return "list_pods" }
func (t *ListPodsTool) Description() string {
	return "列出 Kubernetes Namespace 下的所有 Pod，返回名称、状态、节点、IP 等摘要信息"
}

// 实现一个工具(Tool)的输入参数schema定义
func (t *ListPodsTool) InputSchema() json.RawMessage {
	schema := tools.ParameterSchema{
		Type:        "object",
		Description: t.Description(),
		Required:    []string{"namespace"},
		Properties: map[string]tools.ParamProp{
			"namespace": {Type: "string", Description: "Kubernetes Namespace 名称"},
		},
	}

	b, _ := json.Marshal(schema)
	return b
}

// 查询pod列表返回结构化结果
func (t *ListPodsTool) Execute(ctx context.Context, input json.RawMessage) (tools.ToolResult, error) {
	var params struct {
		Namespace string `json:"namespace"`
	}

	//将输入参数解析为结构体
	if err := json.Unmarshal(input, &params); err != nil {
		return tools.ErrorResult("invalid_input", "参数解析失败", 0), err
	}

	//检查Namespace是否合法
	if err := t.client.CheckNamespace(params.Namespace); err != nil {
		return tools.ErrorResult("namespace_denied", err.Error(), 0), err
	}

	//设置超时的上下文
	ctx, cancel := t.client.TimeoutContext(ctx)
	defer cancel()

	/*
		通过client-go的ClientSet查询指定NameSpace下的所有pod,metav1.ListOptions{} 为空表示不附加过滤条件（查全部）。失败则返回 k8s_api_error,消息里带上具体错误。
	*/
	pods, err := t.client.Clientset.CoreV1().Pods(params.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return tools.ErrorResult("k8s_api_error", fmt.Sprintf("查询 Pod 列表失败: %v", err), 0), err
	}

	type podInfo struct {
		Name     string `json:"name"`
		Status   string `json:"status"`
		Node     string `json:"node"`
		IP       string `json:"ip"`
		Restarts int32  `json:"restarts"`
	}
	result := make([]podInfo, 0, len(pods.Items))
	//外层遍历每个pod
	for _, pod := range pods.Items {
		restarts := int32(0)
		//内核遍历每个pod的容器状态，累加每个容器的 RestartCount，得到这个 Pod 的总重启次数。
		for _, s := range pod.Status.ContainerStatuses {
			restarts += s.RestartCount
		}
		result = append(result, podInfo{
			Name:     pod.Name,
			Status:   string(pod.Status.Phase),
			Node:     pod.Spec.NodeName,
			IP:       pod.Status.PodIP,
			Restarts: restarts,
		})
	}

	//返回成功结果
	return tools.SuccessResult(map[string]any{
		"count": len(result),
		"pods":  result,
	}, "kubernetes", 0), nil
}

// 获取单个pod详细
type GetPodTool struct {
	client *Client
}

func (t *GetPodTool) Name() string { return "get_pod" }
func (t *GetPodTool) Description() string {
	return "获取指定 Pod 的详细信息，包括容器列表、标签等"
}

func (t *GetPodTool) InputSchema() json.RawMessage {
	schema := tools.ParameterSchema{
		Type:        "object",
		Description: t.Description(),
		Required:    []string{"namespace", "name"},
		Properties: map[string]tools.ParamProp{
			"namespace": {Type: "string", Description: "Kubernetes Namespace 名称"},
			"name":      {Type: "string", Description: "Pod 名称"},
		},
	}
	b, _ := json.Marshal(schema)
	return b
}

func (t *GetPodTool) Execute(ctx context.Context, input json.RawMessage) (tools.ToolResult, error) {
	var params struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return tools.ErrorResult("invalid_input", "参数解析失败", 0), err
	}
	if params.Name == "" {
		return tools.ErrorResult("invalid_input", "pod name 是必填项", 0), fmt.Errorf("pod name is required")
	}

	if err := t.client.CheckNamespace(params.Namespace); err != nil {
		return tools.ErrorResult("namespace_denied", err.Error(), 0), err
	}

	ctx, cancel := t.client.TimeoutContext(ctx)
	defer cancel()

	pod, err := t.client.Clientset.CoreV1().Pods(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{})
	if err != nil {
		return tools.ErrorResult("k8s_api_error", fmt.Sprintf("获取 Pod 详情失败: %v", err), 0), err
	}

	type containerInfo struct {
		Name    string `json:"name"`
		Image   string `json:"image"`
		Ready   bool   `json:"ready"`
		Restart int32  `json:"restart"`
	}
	//检查该pod中各个容器的状态
	containers := make([]containerInfo, 0, len(pod.Status.ContainerStatuses))
	for _, cs := range pod.Status.ContainerStatuses {
		containers = append(containers, containerInfo{
			Name:    cs.Name,
			Image:   cs.Image,
			Ready:   cs.Ready,
			Restart: cs.RestartCount,
		})
	}

	return tools.SuccessResult(map[string]any{
		"name":       pod.Name,
		"namespace":  pod.Namespace,
		"status":     string(pod.Status.Phase),
		"node":       pod.Spec.NodeName,
		"ip":         pod.Status.PodIP,
		"labels":     pod.Labels,
		"containers": containers,
	}, "kubernetes", 0), nil
}

// 注册pod的相关工具
func RegisterPodTools(registry *tools.Registry, client *Client) {
	registry.Register(&ListPodsTool{client: client})
	registry.Register(&GetPodTool{client: client})
}
