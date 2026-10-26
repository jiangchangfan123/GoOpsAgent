package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/goopsagent/goopsagent/internal/knowledge"
)

// IndexFile 索引文件结构
type IndexFile struct {
	Chunks []knowledge.Chunk `json:"chunks"`
	Stats  IndexStats        `json:"stats"`
}

// IndexStats 索引统计
type IndexStats struct {
	TotalChunks int `json:"total_chunks"`
	PhaseCounts map[string]int `json:"phase_counts"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: go run cmd/ingest/main.go <知识文档目录> [输出文件]")
		fmt.Println("示例: go run cmd/ingest/main.go knowledge/ internal/knowledge/index.json")
		os.Exit(1)
	}

	inputDir := os.Args[1]
	outputFile := "internal/knowledge/index.json"
	if len(os.Args) > 2 {
		outputFile = os.Args[2]
	}

	// 1. 扫描所有 Markdown 文件
	files, err := filepath.Glob(filepath.Join(inputDir, "*.md"))
	if err != nil {
		log.Fatalf("扫描目录失败: %v", err)
	}

	if len(files) == 0 {
		log.Fatalf("目录 %s 中没有 .md 文件", inputDir)
	}

	fmt.Printf("找到 %d 个文档\n", len(files))

	// 2. 解析文档 → Chunk
	var allChunks []knowledge.Chunk
	phaseCounts := make(map[string]int)

	for _, fp := range files {
		fmt.Printf("  解析：%s\n", filepath.Base(fp))

		data, err := os.ReadFile(fp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  警告：读取 %s 失败: %v\n", fp, err)
			continue
		}

		result, err := knowledge.ParseMarkdown(fp, string(data))
		if err != nil {
			fmt.Fprintf(os.Stderr, "  警告：解析 %s 失败: %v\n", fp, err)
			continue
		}

		fmt.Printf("    → %d 个 Chunk\n", len(result.Chunks))
		for _, c := range result.Chunks {
			allChunks = append(allChunks, c)
			phaseCounts[c.Phase]++
		}
	}

	// 3. 训练 Embedder
	fmt.Printf("\n训练 Embedder（%d 个 Chunk）...\n", len(allChunks))
	emb := knowledge.NewEmbedder()

	// 要训练 Embedder，需要所有文档的原始文本
	// 这里我们收集所有 Chunk 的 Content 作为训练语料
	corpus := make([]string, len(allChunks))
	for i, c := range allChunks {
		corpus[i] = c.Content
	}
	emb.Fit(corpus)
	fmt.Printf("  词汇量：%d\n", emb.Dim())

	// 4. 合并相似 Chunk
	allChunks = knowledge.JoinSimilarChunks(allChunks, 800)
	fmt.Printf("合并后：%d 个 Chunk\n", len(allChunks))

	// 5. 写入索引 JSON
	index := IndexFile{
		Chunks: allChunks,
		Stats: IndexStats{
			TotalChunks: len(allChunks),
			PhaseCounts: phaseCounts,
		},
	}

	// 确保输出目录存在
	outDir := filepath.Dir(outputFile)
	if outDir != "." {
		if err := os.MkdirAll(outDir, 0755); err != nil {
			log.Fatalf("创建输出目录失败: %v", err)
		}
	}

	jsonData, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		log.Fatalf("JSON 序列化失败: %v", err)
	}

	if err := os.WriteFile(outputFile, jsonData, 0644); err != nil {
		log.Fatalf("写入索引文件失败: %v", err)
	}

	fmt.Printf("\n✅ 索引已写入 %s\n", outputFile)
	fmt.Printf("   总 Chunk 数：%d\n", len(allChunks))
	fmt.Println("\n各阶段分布：")
	phaseOrder := []string{"scheduling", "image", "startup", "runtime", "network", "general"}
	for _, phase := range phaseOrder {
		if count, ok := phaseCounts[phase]; ok {
			fmt.Printf("   %-15s %d\n", phase, count)
		}
	}

	// 6. 额外生成 Embedder 训练信息（供 server 使用，非必须）
	embInfo := fmt.Sprintf("vocab_size=%d\n", emb.Dim())
	embFile := strings.Replace(outputFile, ".json", "_emb.txt", 1)
	_ = os.WriteFile(embFile, []byte(embInfo), 0644)
}