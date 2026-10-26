package agent

import (
	"fmt"
	"strings"

	"github.com/goopsagent/goopsagent/internal/knowledge"
	"github.com/goopsagent/goopsagent/internal/tools"
)

// BuildSystemPrompt 构造系统提示词（无 RAG）
func BuildSystemPrompt(toolDefs []tools.ToolDef) string {
	return BuildSystemPromptWithRAG(toolDefs, nil)
}

// BuildSystemPromptWithRAG 构造系统提示词（带 RAG 上下文）
func BuildSystemPromptWithRAG(toolDefs []tools.ToolDef, ragResults []knowledge.SearchResult) string {
	var sb strings.Builder

	sb.WriteString("你是一个 Kubernetes 运维助手。")
	sb.WriteString("你的任务是帮助用户诊断和查询 Kubernetes 集群状态。\n\n")
	sb.WriteString("规则：\n")
	sb.WriteString("1. 使用提供的工具获取真实数据，不要编造。\n")
	sb.WriteString("2. 每次只调用一个工具。\n")
	sb.WriteString("3. 分析工具返回的数据后再给结论。\n")
	sb.WriteString("4. 如果工具返回错误，如实告知用户。\n")
	sb.WriteString("5. 区分事实和推断：引用工具返回的具体数据作为证据。\n")

	// RAG 上下文：注入检索到的相关知识
	if len(ragResults) > 0 {
		sb.WriteString("\n=== 参考知识库 ===\n")
		sb.WriteString("以下是从 Kubernetes 故障排查知识库中检索到的相关内容，")
		sb.WriteString("请优先参考这些知识进行诊断：\n\n")
		for i, r := range ragResults {
			sb.WriteString(fmt.Sprintf("【参考 %d】(相似度 %.0f%%)\n", i+1, r.Similarity*100))
			sb.WriteString(r.Chunk.Content)
			sb.WriteString("\n\n")
		}
		sb.WriteString("=== 知识库引用结束 ===\n\n")
		sb.WriteString("注意：以上知识库内容仅供参考。你必须结合实际集群数据（通过工具获取）")
		sb.WriteString("来验证和修正诊断结论。当知识库建议与集群实际状态不一致时，以集群实际数据为准。\n\n")
	}

	sb.WriteString("可用工具：\n")
	for _, td := range toolDefs {
		sb.WriteString(fmt.Sprintf("- %s：%s\n", td.Name, td.Description))
	}

	sb.WriteString("\n当你完成所有排查后，必须在最终回复中输出一个 JSON 格式的诊断报告，")
	sb.WriteString("字段包括：summary（一句话摘要）、root_cause（根因分析）、confidence（high/medium/low）、")
	sb.WriteString("evidence（数组，每项含 source/content/relevance）、suggestions（建议操作列表）。")
	sb.WriteString("如果信息不足以确定根因，confidence 设为 low 并在 summary 中说明。")

	return sb.String()
}
