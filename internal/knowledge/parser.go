package knowledge

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ParseResult 解析一篇 Markdown 文档的结果
type ParseResult struct {
	Document *Document
	Chunks   []Chunk
}

// inferPhase 从标题中推断排查阶段
func inferPhase(title string) string {
	lower := strings.ToLower(title)
	switch {
	case strings.Contains(lower, "调度") || strings.Contains(lower, "pending") || strings.Contains(lower, "scheduling"):
		return "scheduling"
	case strings.Contains(lower, "镜像") || strings.Contains(lower, "image") || strings.Contains(lower, "pull"):
		return "image"
	case strings.Contains(lower, "启动") || strings.Contains(lower, "crash") || strings.Contains(lower, "startup"):
		return "startup"
	case strings.Contains(lower, "运行") || strings.Contains(lower, "oom") || strings.Contains(lower, "runtime") || strings.Contains(lower, "evict"):
		return "runtime"
	case strings.Contains(lower, "网络") || strings.Contains(lower, "network") || strings.Contains(lower, "service") || strings.Contains(lower, "dns"):
		return "network"
	default:
		return "general"
	}
}

// ParseMarkdown 解析一篇 Markdown 知识文档
// 按 ## 级标题切分为 Chunk，每个 Chunk 包含所属标题层级的上下文
func ParseMarkdown(filePath, rawContent string) (*ParseResult, error) {
	lines := strings.Split(rawContent, "\n")

	docID := uuid.New().String()
	docTitle := ""

	// 提取文档主标题（第一个 # 行）
	for _, line := range lines {
		if strings.HasPrefix(line, "# ") {
			docTitle = strings.TrimPrefix(line, "# ")
			break
		}
	}
	if docTitle == "" {
		docTitle = filePath
	}

	document := &Document{
		ID:        docID,
		Title:     docTitle,
		Content:   rawContent,
		Source:    filePath,
		Phase:     inferPhase(docTitle),
		CreatedAt: time.Now(),
	}

	// 按 ## 级标题切分 Chunk
	type section struct {
		title   string
		content []string
	}
	var sections []section
	var sectionTitle string
	var sectionContent []string

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// 检测 H2 标题
		if strings.HasPrefix(line, "## ") {
			// 保存上一段
			if len(sectionContent) > 0 {
				sections = append(sections, section{
					title:   sectionTitle,
					content: sectionContent,
				})
			}
			// 开始新段
			sectionTitle = strings.TrimPrefix(line, "## ")
			sectionContent = []string{line}
		} else if sectionTitle != "" {
			//如果存在子标题，则h2标题以下的内容会添加到sectioncontent中
			sectionContent = append(sectionContent, line)
		}

		if i == len(lines)-1 && len(sectionContent) > 0 {
			sections = append(sections, section{
				title:   sectionTitle,
				content: sectionContent,
			})
		}
	}

	// 没有子标题，整个文档作为一个 Chunk
	if len(sections) == 0 {
		chunk := Chunk{
			ID:        uuid.New().String(),
			DocID:     docID,
			Title:     docTitle,
			Content:   rawContent,
			Phase:     document.Phase,
			Index:     0,
			CreatedAt: time.Now(),
		}
		return &ParseResult{Document: document, Chunks: []Chunk{chunk}}, nil
	}

	// 为每个 section 创建 Chunk
	var chunks []Chunk
	for idx, sec := range sections {
		content := strings.Join(sec.content, "\n")
		// 截断过长的内容（保留前 3000 字符）
		if len(content) > 4000 {
			content = content[:4000]
		}

		chunks = append(chunks, Chunk{
			ID:        uuid.New().String(),
			DocID:     docID,
			Title:     fmt.Sprintf("%s → %s", docTitle, sec.title),
			Content:   content,
			Phase:     inferPhase(sec.title),
			Index:     idx,
			CreatedAt: time.Now(),
		})
	}

	return &ParseResult{
		Document: document,
		Chunks:   chunks,
	}, nil
}

// EstimateTokens 粗略估算 token 数（英文 1 token ≈ 4 字符；中文 1 token ≈ 1.5 字符）
func EstimateTokens(text string) int {
	chars := 0
	chineseChars := 0
	for _, r := range text {
		if r >= 0x4e00 && r <= 0x9fff {
			chineseChars++
		} else {
			chars++
		}
	}
	// 中文每 1.5 字符 1 token，英文每 4 字符 1 token
	est := int(math.Ceil(float64(chars)/4.0 + float64(chineseChars)/1.5))
	return est
}

// JoinSimilarChunks 合并同一 Phase 中相似度过高的相邻 Chunk
// step: 合并后 Chunk 的最大 token 数（粗略估算）
func JoinSimilarChunks(chunks []Chunk, maxTokens int) []Chunk {
	if len(chunks) <= 1 || maxTokens <= 0 {
		return chunks
	}

	// 按 Phase(阶段) 分组
	groups := make(map[string][]Chunk)
	for _, c := range chunks {
		groups[c.Phase] = append(groups[c.Phase], c)
	}

	var result []Chunk
	for _, group := range groups {
		var merged []Chunk
		current := group[0]
		for i := 1; i < len(group); i++ {
			next := group[i]
			combinedTokens := EstimateTokens(current.Content) + EstimateTokens(next.Content)
			if combinedTokens <= maxTokens {
				// 合并
				current.Content = current.Content + "\n\n" + next.Content
				current.ID = next.ID // 用后者 ID，方便追溯
			} else {
				merged = append(merged, current)
				current = next
			}
		}
		merged = append(merged, current)
		result = append(result, merged...)
	}

	return result
}
