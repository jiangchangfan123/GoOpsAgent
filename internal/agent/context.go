package agent

import (
	"fmt"
	"strings"

	"github.com/goopsagent/goopsagent/internal/tools"
)

// BuildSystemPrompt 构造系统提示词
func BuildSystemPrompt(toolDefs []tools.ToolDef) string {
	var sb strings.Builder

	sb.WriteString("你是一个 Kubernetes 运维助手。")
	sb.WriteString("你的任务是帮助用户诊断和查询 Kubernetes 集群状态。\n\n")
	sb.WriteString("规则：\n")
	sb.WriteString("1. 使用提供的工具获取真实数据，不要编造。\n")
	sb.WriteString("2. 每次只调用一个工具。\n")
	sb.WriteString("3. 分析工具返回的数据后再给结论。\n")
	sb.WriteString("4. 如果工具返回错误，如实告知用户。\n")
	sb.WriteString("5. 区分事实和推断：引用工具返回的具体数据作为证据。\n\n")

	sb.WriteString("可用工具：\n")
	for _, td := range toolDefs {
		sb.WriteString(fmt.Sprintf("- %s：%s\n", td.Name, td.Description))
	}

	return sb.String()
}
