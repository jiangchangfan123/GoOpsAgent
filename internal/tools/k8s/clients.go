package k8s

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

//InCluster 模式:你的 GoOpsAgent 程序本身就跑在 K8s 集群里（作为一个 Pod）。
//Kubeconfig 模式:你的 GoOpsAgent 程序跑在 K8s 集群外，比如你的本地开发环境，或者你的 CI/CD 环境等。

type ClientConfig struct {
	Mode               string // "incluster" 或 "kubeconfig"
	KubeconfigPath     string
	Timeout            time.Duration // 每次 API 调用的默认超时
	NamespaceWhitelist []string      // 允许访问的命名空间
}

// Client 封装 Kubernetes clientset
type Client struct {
	Clientset          *kubernetes.Clientset
	config             *rest.Config
	namespaceWhitelist []string
	defaultTimeout     time.Duration
}

// 创建k8s客户端(incluster或kubeconfig两种模式)
func NewClient(cfg ClientConfig) (*Client, error) {
	var restCfg *rest.Config
	var err error

	switch cfg.Mode {
	case "incluster":
		restCfg, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("incluster config: %w", err)
		}
		slog.Info("k8s client: using in-cluster config")
	case "kubeconfig":
		loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: cfg.KubeconfigPath}
		clientCfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			loadingRules, &clientcmd.ConfigOverrides{},
		)
		restCfg, err = clientCfg.ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("kubeconfig: %w", err)
		}
		slog.Info("k8s client: using kubeconfig", "path", cfg.KubeconfigPath)
	default:
		return nil, fmt.Errorf("unknown k8s mode: %q (expect incluster or kubeconfig)", cfg.Mode)
	}

	restCfg.Timeout = cfg.Timeout
	clientset, err := kubernetes.NewForConfig(restCfg)

	if err != nil {
		return nil, fmt.Errorf("create clientset: %w", err)
	}

	return &Client{
		Clientset:          clientset,
		config:             restCfg,
		namespaceWhitelist: cfg.NamespaceWhitelist,
		defaultTimeout:     cfg.Timeout,
	}, nil
}

// CheckNamespace 校验命名空间是否在白名单内
func (c *Client) CheckNamespace(namespace string) error {
	if len(c.namespaceWhitelist) == 0 {
		return nil // 没有白名单 = 允许所有
	}
	for _, ns := range c.namespaceWhitelist {
		if ns == namespace {
			return nil
		}
	}
	return fmt.Errorf("namespace %q is not in the whitelist", namespace)
}

// TimeoutContext 返回带超时的 context
func (c *Client) TimeoutContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, c.defaultTimeout)
}
