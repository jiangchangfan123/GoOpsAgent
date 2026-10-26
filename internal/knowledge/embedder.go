package knowledge

import (
	"math"
	"sort"
	"strings"
)

// Embedder 轻量级文本嵌入器
// 使用 TF-IDF 词袋模型，无需外部 ML 依赖
// 向量维度 = vocabularySize（训练时自动确定）
type Embedder struct {
	vocabulary  map[string]int // 词 → 索引
	idf         []float64      // 逆文档频率
	docCount    int            // 训练语料文档数
	initialized bool
}

// NewEmbedder 创建嵌入器
func NewEmbedder() *Embedder {
	return &Embedder{
		vocabulary: make(map[string]int),
	}
}

// Fit 在语料库上训练，建立词汇表和 IDF
// docs 为所有文档的文本内容
func (e *Embedder) Fit(docs []string) {
	// 第一遍：统计文档频率（每个词在多少篇文档中出现）
	docFreq := make(map[string]int)
	for _, doc := range docs {
		seen := make(map[string]bool)
		for _, word := range tokenize(doc) {
			if !seen[word] {
				docFreq[word]++
				seen[word] = true
			}
		}
	}

	// 第二遍：构建词汇表（过滤只出现 1 次的罕见词，减少噪声）
	e.vocabulary = make(map[string]int, len(docFreq))
	e.idf = make([]float64, 0, len(docFreq))
	e.docCount = len(docs)
	if e.docCount == 0 {
		e.docCount = 1
	}

	for word, df := range docFreq {
		if df < 2 && len(docs) > 5 {
			continue // 罕见词过滤
		}
		idx := len(e.idf)
		e.vocabulary[word] = idx
		idf := math.Log(float64(e.docCount) / float64(df))
		e.idf = append(e.idf, idf)
	}

	e.initialized = true
}

// Embed 将单段文本转为稠密向量
func (e *Embedder) Embed(text string) []float32 {
	if !e.initialized {
		return nil
	}

	tokens := tokenize(text)
	if len(tokens) == 0 {
		return make([]float32, len(e.idf))
	}

	// 计算词频
	tf := make(map[string]float64)
	for _, t := range tokens {
		tf[t]++
	}
	maxTF := 0.0
	for _, f := range tf {
		if f > maxTF {
			maxTF = f
		}
	}
	// 归一化词频 (augmented TF)
	for w := range tf {
		tf[w] = 0.5 + 0.5*(tf[w]/maxTF)
	}

	// TF-IDF 向量
	vec := make([]float32, len(e.idf))
	var norm float64
	for w, idx := range e.vocabulary {
		val := tf[w] * e.idf[idx]
		vec[idx] = float32(val)
		norm += val * val
	}

	// L2 归一化
	if norm > 0 {
		norm = math.Sqrt(norm)
		for i := range vec {
			vec[i] = float32(float64(vec[i]) / norm)
		}
	}

	return vec
}

// CosineSimilarity 计算两个向量的余弦相似度
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		da := float64(a[i])
		db := float64(b[i])
		dot += da * db
		normA += da * da
		normB += db * db
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// Dim 返回向量维度
func (e *Embedder) Dim() int {
	return len(e.idf)
}

// tokenize 中文/英文简单分词
func tokenize(text string) []string {
	// 转小写
	text = strings.ToLower(text)

	// 按空白字符和标点分割
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' ||
			r == ',' || r == '.' || r == ';' || r == ':' || r == '!' ||
			r == '?' || r == '(' || r == ')' || r == '[' || r == ']' ||
			r == '{' || r == '}' || r == '"' || r == '\'' || r == '`' ||
			r == '|' || r == '/' || r == '\\' || r == '-' || r == '_' ||
			r == '+' || r == '=' || r == '<' || r == '>' || r == '@' ||
			r == '#' || r == '$' || r == '%' || r == '^' || r == '&' ||
			r == '*'
	})

	// 过滤停用词和太短的词
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "can": true, "shall": true, "to": true,
		"of": true, "in": true, "for": true, "on": true, "with": true,
		"at": true, "by": true, "from": true, "as": true, "into": true,
		"through": true, "during": true, "before": true, "after": true,
		"above": true, "below": true, "between": true, "and": true, "but": true,
		"or": true, "not": true, "no": true, "if": true, "then": true,
		"else": true, "when": true, "where": true, "why": true, "how": true,
		"it": true, "its": true, "this": true, "that": true, "these": true,
		"those": true, "i": true, "me": true, "my": true, "we": true,
		"our": true, "you": true, "your": true, "he": true, "she": true,
		"him": true, "her": true, "they": true, "them": true, "their": true,
		"的": true, "了": true, "在": true, "是": true, "与": true,
		"和": true, "或": true, "等": true, "及": true, "其": true,
		"该": true, "此": true, "本": true, "各": true, "为": true,
	}

	// 按长度排序 + 去重后返回
	seen := make(map[string]bool)
	result := make([]string, 0, len(fields))
	for _, f := range fields {
		// 跳过停用词、空词、太短的词
		if len(f) < 2 || stopWords[f] || seen[f] {
			continue
		}
		// 跳过纯数字
		if isDigitsOnly(f) {
			continue
		}
		seen[f] = true
		result = append(result, f)
	}

	// 按词频排序保留高频词（TopK 可选但这里全保留）
	sort.Strings(result)
	return result
}

// 判断是否只是数字
func isDigitsOnly(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}
