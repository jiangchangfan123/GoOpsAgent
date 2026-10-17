package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// 客户端模型配置
type Config struct {
	APIKey    string
	BaseURL   string
	Model     string
	Timeout   time.Duration
	MaxTokens int
}

// Client LLM 客户端
type Client struct {
	cfg    Config
	client *http.Client
}

// 创建LLM客户端
func NewClient(cfg Config) *Client {
	return &Client{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// chat发送聊天请求
func (c *Client) Chat(ctx context.Context, message []ChatMessage) (*ChatResponse, error) {
	reqBody := ChatRequest{
		Model:     c.cfg.Model,
		Messages:  message,
		MaxTokens: c.cfg.MaxTokens,
	}

	resp, err := c.DoChat(ctx, reqBody)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) ChatStream(ctx context.Context, message []ChatMessage) (<-chan ChatMessage, <-chan error) {
	msgch := make(chan ChatMessage, 10)
	errch := make(chan error, 1)

	go func() {
		defer close(msgch)
		defer close(errch)

		reqBody := ChatRequest{
			Model:     c.cfg.Model,
			Messages:  message,
			MaxTokens: c.cfg.MaxTokens,
			Stream:    true,
		}

		body, err := json.Marshal(reqBody)
		if err != nil {
			errch <- fmt.Errorf("marshal request: %w", err)
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx,
			http.MethodPost,
			c.cfg.BaseURL+"/chat/completions",
			bytes.NewReader(body),
		)
		if err != nil {
			errch <- fmt.Errorf("create request: %w", err)
			return
		}

		c.setHeaders(httpReq)

		httpResp, err := c.client.Do(httpReq)
		if err != nil {
			errch <- classifyError(err)
			return
		}
		defer httpResp.Body.Close()

		if httpResp.StatusCode != http.StatusOK {
			errch <- c.parseError(httpResp)
			return
		}

		//解析SSE流式响应
		decoder := json.NewDecoder(httpResp.Body)
		for {
			select {
			case <-ctx.Done():
				errch <- ctx.Err()
				return
			default:
			}

			var chunk struct {
				Choices []struct {
					Delta struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"delta"`
					FinishReason *string `json:"finish_reason"`
				} `json:"choices"`
			}

			if err := decoder.Decode(&chunk); err != nil {
				if err == io.EOF {
					return
				}
				errch <- fmt.Errorf("decode response: %w", err)
				return
			}

			if len(chunk.Choices) > 0 {
				msgch <- ChatMessage{
					Role:    chunk.Choices[0].Delta.Role,
					Content: chunk.Choices[0].Delta.Content,
				}
				if chunk.Choices[0].FinishReason != nil {
					return
				}
			}
		}
	}()
	return msgch, errch
}

func (c *Client) DoChat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx,
		http.MethodPost,
		c.cfg.BaseURL+"chat/completions",
		bytes.NewReader(body),
	)

	if err != nil {
		return nil, fmt.Errorf("create http request: %w", err)
	}

	c.setHeaders(httpReq)

	start := time.Now()
	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, classifyError(err)
	}
	defer httpResp.Body.Close()

	slog.Info("llm call",
		"model", c.cfg.Model,
		"status", httpResp.StatusCode,
		"duration_ms", time.Since(start).Milliseconds(),
	)

	if httpResp.StatusCode != http.StatusOK {
		return nil, c.parseError(httpResp)
	}

	var resp ChatResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &resp, nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
}

func (c *Client) parseError(resp *http.Response) *LLMError {
	body, _ := io.ReadAll(resp.Body)
	llmErr := &LLMError{
		StatusCode: resp.StatusCode,
	}

	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		llmErr.Message = "rate limited"
		llmErr.Retryable = true
	case http.StatusUnauthorized, http.StatusForbidden:
		llmErr.Message = "authentication failed"
	default:
		if resp.StatusCode >= 500 {
			llmErr.Message = fmt.Sprintf("server error: %s", string(body))
			llmErr.Retryable = true
		} else {
			llmErr.Message = fmt.Sprintf("request failed: %s", string(body))
		}
	}

	// 不在日志中输出完整 body（可能含密钥信息）
	slog.Error("llm error",
		"status_code", resp.StatusCode,
		"retryable", llmErr.Retryable,
	)
	return llmErr
}

func classifyError(err error) *LLMError {
	if errors.Is(err, context.DeadlineExceeded) {
		return &LLMError{
			Message:   "request timeout",
			Retryable: true,
		}
	}
	if errors.Is(err, context.Canceled) {
		return &LLMError{
			Message: "request cancelled",
		}
	}
	return &LLMError{
		Message:   fmt.Sprintf("network error: %v", err),
		Retryable: true,
	}
}
