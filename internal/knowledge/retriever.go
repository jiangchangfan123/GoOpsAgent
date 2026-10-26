package knowledge

import (
	"sort"
	"sync"
)

// Retriever 知识检索器
// 维护所有 Chunk 及其向量的内存索引，提供语义搜索
type Retriever struct {
	mu         sync.RWMutex
	chunks     []Chunk
	embeddings [][]float32 // chunks[i] 的向量
	embedder   *Embedder
	config     RetrieverConfig
}

// NewRetriever 创建检索器
func NewRetriever(embedder *Embedder, config RetrieverConfig) *Retriever {
	if config.MaxResults <= 0 {
		config.MaxResults = 5
	}
	if config.MinScore <= 0 {
		config.MinScore = 0.3
	}
	return &Retriever{
		embedder: embedder,
		config:   config,
	}
}

// Index 将 Chunk 列表建立索引（替换现有索引）
func (r *Retriever) Index(chunks []Chunk) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.chunks = make([]Chunk, len(chunks))
	copy(r.chunks, chunks)

	r.embeddings = make([][]float32, len(chunks))
	for i, c := range chunks {
		r.embeddings[i] = r.embedder.Embed(c.Content)
	}
}

// Search 语义搜索
// query: 用户查询文本
// topK: 返回条数；<=0 使用默认配置
// phaseFilter: 可选，按 Phase 过滤（空字符串不过滤）
func (r *Retriever) Search(query string, topK int, phaseFilter string) []SearchResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if topK <= 0 {
		topK = r.config.MaxResults
	}

	if len(r.chunks) == 0 {
		return nil
	}

	// 将查询转为向量
	queryVec := r.embedder.Embed(query)
	if queryVec == nil || len(queryVec) == 0 {
		return nil
	}

	// 计算与所有 Chunk 的相似度
	type scored struct {
		idx  int
		sim  float64
	}
	var scores []scored

	for i, emb := range r.embeddings {
		if emb == nil {
			continue
		}
		// Phase 过滤
		if phaseFilter != "" && r.chunks[i].Phase != "" && r.chunks[i].Phase != phaseFilter {
			continue
		}
		sim := CosineSimilarity(queryVec, emb)
		if sim >= r.config.MinScore {
			scores = append(scores, scored{idx: i, sim: sim})
		}
	}

	// 按相似度降序
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].sim > scores[j].sim
	})

	// 取 topK
	if len(scores) > topK {
		scores = scores[:topK]
	}

	results := make([]SearchResult, len(scores))
	for i, s := range scores {
		results[i] = SearchResult{
			Chunk:      r.chunks[s.idx],
			Similarity: s.sim,
		}
	}

	return results
}

// ChunkCount 返回当前索引的 Chunk 数
func (r *Retriever) ChunkCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.chunks)
}

// GetConfig 返回配置
func (r *Retriever) GetConfig() RetrieverConfig {
	return r.config
}