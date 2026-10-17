package provider

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest 聊天请求
type ChatRequest struct {
	Model     string        `json:"model"`
	Messages  []ChatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens,omitempty"`
	Stream    bool          `json:"stream,omitempty"`
}

type ChatResponse struct {
	ID      string       `json:"id"`
	Choices []ChatChoice `json:"choices"`
	Usage   *UsageInfo   `json:"usage,omitempty"`
}

type ChatChoice struct {
	Index   int         `json:"index"`
	Message ChatMessage `json:"message"`
}

type UsageInfo struct {
	PromptsTokens     int `json:"prompt_tokens"`
	ComplietionTokens int `json:"completion_tokens"`
	TotalTokens       int `json:"total_tokens"`
}

// LLMError 模型调用错误
type LLMError struct {
	Message    string `json:"message"`
	StatusCode int    `json:"status_code,omitempty"`
	Retryable  bool   `json:"retryable"`
}

func (e *LLMError) Error() string {
	return e.Message
}
