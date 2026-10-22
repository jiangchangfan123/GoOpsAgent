package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/goopsagent/goopsagent/internal/tools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ---- list_deployments：列出 Namespace 下所有 Deployment ----

type ListDeploymentsTool struct {
	client *Client
}

func (t *ListDeploymentsTool) Name() string { return "list_deployments" }
func (t *ListDeploymentsTool) Description() string {
	return "列出 Kubernetes Namespace 下的所有 Deployment，返回名称、副本数、镜像等摘要"
}

func (t *ListDeploymentsTool) InputSchema() json.RawMessage {
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

func (t *ListDeploymentsTool) Execute(ctx context.Context, input json.RawMessage) (tools.ToolResult, error) {
	var params struct {
		Namespace string `json:"namespace"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return tools.ErrorResult("invalid_input", "参数解析失败", 0), err
	}

	if err := t.client.CheckNamespace(params.Namespace); err != nil {
		return tools.ErrorResult("namespace_denied", err.Error(), 0), err
	}

	ctx, cancel := t.client.TimeoutContext(ctx)
	defer cancel()

	deployments, err := t.client.Clientset.AppsV1().Deployments(params.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return tools.ErrorResult("k8s_api_error", fmt.Sprintf("查询 Deployment 列表失败: %v", err), 0), err
	}

	type deployInfo struct {
		Name       string   `json:"name"`
		Replicas   int32    `json:"replicas"` //副本数
		Ready      int32    `json:"ready"`
		Available  int32    `json:"available"`
		Containers []string `json:"containers"` //一个需要运行的pod里所包含的容器
	}
	result := make([]deployInfo, 0, len(deployments.Items))
	for _, d := range deployments.Items {
		images := make([]string, 0, len(d.Spec.Template.Spec.Containers))
		for _, c := range d.Spec.Template.Spec.Containers {
			images = append(images, c.Image)
		}
		result = append(result, deployInfo{
			Name:       d.Name,
			Replicas:   *d.Spec.Replicas,
			Ready:      d.Status.ReadyReplicas,
			Available:  d.Status.AvailableReplicas,
			Containers: images,
		})
	}

	return tools.SuccessResult(map[string]any{
		"count":       len(result),
		"deployments": result,
	}, "kubernetes", 0), nil
}

// ---- get_deployment：获取单个 Deployment 详情 ----

type GetDeploymentTool struct {
	client *Client
}

func (t *GetDeploymentTool) Name() string { return "get_deployment" }
func (t *GetDeploymentTool) Description() string {
	return "获取指定 Deployment 的详细信息，包含容器镜像和资源配置"
}

func (t *GetDeploymentTool) InputSchema() json.RawMessage {
	schema := tools.ParameterSchema{
		Type:        "object",
		Description: t.Description(),
		Required:    []string{"namespace", "name"},
		Properties: map[string]tools.ParamProp{
			"namespace": {Type: "string", Description: "Kubernetes Namespace 名称"},
			"name":      {Type: "string", Description: "Deployment 名称"},
		},
	}
	b, _ := json.Marshal(schema)
	return b
}

func (t *GetDeploymentTool) Execute(ctx context.Context, input json.RawMessage) (tools.ToolResult, error) {
	var params struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return tools.ErrorResult("invalid_input", "参数解析失败", 0), err
	}
	if params.Name == "" {
		return tools.ErrorResult("invalid_input", "deployment name 是必填项", 0), fmt.Errorf("deployment name is required")
	}

	if err := t.client.CheckNamespace(params.Namespace); err != nil {
		return tools.ErrorResult("namespace_denied", err.Error(), 0), err
	}

	ctx, cancel := t.client.TimeoutContext(ctx)
	defer cancel()

	d, err := t.client.Clientset.AppsV1().Deployments(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{})
	if err != nil {
		return tools.ErrorResult("k8s_api_error", fmt.Sprintf("获取 Deployment 详情失败: %v", err), 0), err
	}

	type containerDetail struct {
		Name       string `json:"name"`
		Image      string `json:"image"`
		CPURequest string `json:"cpu_request"`
		MemRequest string `json:"mem_request"`
		CPULimit   string `json:"cpu_limit"`
		MemLimit   string `json:"mem_limit"`
	}
	containers := make([]containerDetail, 0, len(d.Spec.Template.Spec.Containers))
	for _, c := range d.Spec.Template.Spec.Containers {
		detail := containerDetail{Name: c.Name, Image: c.Image}
		if c.Resources.Requests.Cpu() != nil {
			detail.CPURequest = c.Resources.Requests.Cpu().String()
		}
		if c.Resources.Requests.Memory() != nil {
			detail.MemRequest = c.Resources.Requests.Memory().String()
		}
		if c.Resources.Limits.Cpu() != nil {
			detail.CPULimit = c.Resources.Limits.Cpu().String()
		}
		if c.Resources.Limits.Memory() != nil {
			detail.MemLimit = c.Resources.Limits.Memory().String()
		}
		containers = append(containers, detail)
	}

	return tools.SuccessResult(map[string]any{
		"name":       d.Name,
		"namespace":  d.Namespace,
		"replicas":   *d.Spec.Replicas,
		"ready":      d.Status.ReadyReplicas,
		"available":  d.Status.AvailableReplicas,
		"labels":     d.Spec.Template.Labels,
		"containers": containers,
	}, "kubernetes", 0), nil
}

// RegisterDeploymentTools 一次注册所有 Deployment 工具
func RegisterDeploymentTools(registry *tools.Registry, client *Client) {
	registry.Register(&ListDeploymentsTool{client: client})
	registry.Register(&GetDeploymentTool{client: client})
}
