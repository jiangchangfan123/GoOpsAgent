package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Server     ServerConfig
	Database   DatabaseConfig
	LLM        LLMConfig
	Agent      AgentConfig
	Kubernetes KubernetesConfig
	Redis      RedisConfig
	Log        LogConfig
}

type ServerConfig struct {
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

type DatabaseConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type LLMConfig struct {
	APIKey    string
	BaseURL   string
	Model     string
	Timeout   time.Duration
	MaxTokens int
}

type AgentConfig struct {
	MaxIterations int
	MaxToolCalls  int
	TaskTimeout   time.Duration
	ToolTimeout   time.Duration
}

type KubernetesConfig struct {
	KubeConfig         string
	NamespaceWhitelist []string
}

type RedisConfig struct {
	Addr     string
	Password string
}

type LogConfig struct {
	Level  string
	Format string
}

// Load 从环境变量加载配置并校验
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:            getEnvInt("GOOPSAGENT_SERVER_PORT", 8080),
			ReadTimeout:     getEnvDuration("GOOPSAGENT_SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout:    getEnvDuration("GOOPSAGENT_SERVER_WRITE_TIMEOUT", 60*time.Second),
			ShutdownTimeout: getEnvDuration("GOOPSAGENT_SERVER_SHUTDOWN_TIMEOUT", 10*time.Second),
		},
		Database: DatabaseConfig{
			DSN:             getEnv("GOOPSAGENT_DATABASE_DSN", ""),
			MaxOpenConns:    getEnvInt("GOOPSAGENT_DATABASE_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvInt("GOOPSAGENT_DATABASE_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvDuration("GOOPSAGENT_DATABASE_CONN_MAX_LIFETIME", 5*time.Minute),
		},
		LLM: LLMConfig{
			APIKey:    getEnv("GOOPSAGENT_LLM_API_KEY", ""),
			BaseURL:   getEnv("GOOPSAGENT_LLM_BASE_URL", "https://dashscope.aliyuncs.com/compatible-mode/v1"),
			Model:     getEnv("GOOPSAGENT_LLM_MODEL", "qwen-plus"),
			Timeout:   getEnvDuration("GOOPSAGENT_LLM_TIMEOUT", 60*time.Second),
			MaxTokens: getEnvInt("GOOPSAGENT_LLM_MAX_TOKENS", 4096),
		},
		Agent: AgentConfig{
			MaxIterations: getEnvInt("GOOPSAGENT_AGENT_MAX_ITERATIONS", 10),
			MaxToolCalls:  getEnvInt("GOOPSAGENT_AGENT_MAX_TOOL_CALLS", 20),
			TaskTimeout:   getEnvDuration("GOOPSAGENT_AGENT_TASK_TIMEOUT", 300*time.Second),
			ToolTimeout:   getEnvDuration("GOOPSAGENT_AGENT_TOOL_TIMEOUT", 30*time.Second),
		},
		Kubernetes: KubernetesConfig{
			KubeConfig: getEnv("GOOPSAGENT_K8S_KUBECONFIG", ""),
			NamespaceWhitelist: parseNamespaceWhitelist(
				getEnv("GOOPSAGENT_K8S_NAMESPACE_WHITELIST", "default"),
			),
		},
		Redis: RedisConfig{
			Addr:     getEnv("GOOPSAGENT_REDIS_ADDR", "localhost:6379"),
			Password: getEnv("GOOPSAGENT_REDIS_PASSWORD", ""),
		},
		Log: LogConfig{
			Level:  getEnv("GOOPSAGENT_LOG_LEVEL", "info"),
			Format: getEnv("GOOPSAGENT_LOG_FORMAT", "json"),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	if c.LLM.APIKey == "" {
		return fmt.Errorf("GOOPSAGENT_LLM_API_KEY is required")
	}
	if c.LLM.BaseURL == "" {
		return fmt.Errorf("GOOPSAGENT_LLM_BASE_URL is required")
	}
	if c.Agent.MaxIterations < 1 {
		return fmt.Errorf("GOOPSAGENT_AGENT_MAX_ITERATIONS must be >= 1")
	}
	return nil
}

//------辅助函数---------

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return n
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return defaultVal
	}
	return d
}

func parseNamespaceWhitelist(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
