package knowledge

import "time"

// Document 知识库中的一篇文档
type Document struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Source    string    `json:"source"`    // 来源文件路径
	Phase     string    `json:"phase"`     // 排查阶段：scheduling/image/startup/runtime/network
	Embedding []float32 `json:"embedding"` // 文档向量
	CreatedAt time.Time `json:"created_at"`
}

// Chunk 文档分块，RAG 检索的最小单位
type Chunk struct {
	ID       string    `json:"id"`
	DocID    string    `json:"doc_id"`
	Title    string    `json:"title"`    // 继承自 Document
	Content  string    `json:"content"`  // 分块文本
	Phase    string    `json:"phase"`    // 继承自 Document
	Index    int       `json:"index"`    // 在文档中的序号
	CreatedAt time.Time `json:"created_at"`
}

// SearchResult 检索结果
type SearchResult struct {
	Chunk      Chunk   `json:"chunk"`
	Similarity float64 `json:"similarity"` // 余弦相似度 [0, 1]
}

// RetrieverConfig 检索器配置
type RetrieverConfig struct {
	MaxResults int     `json:"max_results"` // 最大返回条数，默认 5
	MinScore   float64 `json:"min_score"`   // 最小相似度阈值，默认 0.3
}