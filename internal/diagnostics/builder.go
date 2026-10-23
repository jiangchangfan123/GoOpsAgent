package diagnostics

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/goopsagent/goopsagent/internal/models"
)

// BuildReport 从 Task 中构建结构化诊断报告
// 规则：
//   - 优先从 task.Answer 中提取 JSON 格式的诊断报告
//   - 如果 JSON 解析失败，退化为纯文本摘要
//   - 从 task.ToolRuns 中提取证据
func BuildReport(task *models.Task) *models.Report {
	report := &models.Report{
		Confidence:  "low",
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// 1. 尝试从 answer 中提取 JSON 报告
	if raw := extractReportJSON(task.Answer); raw != nil {
		parseReportJSON(raw, report)
	} else {
		// 退化为纯文本摘要
		report.Summary = task.Answer
		report.RootCause = "（未能从模型回复中提取结构化报告，请查看原始回答）"
	}

	// 2. 从工具调用记录中提取证据
	report.Evidence = extractEvidence(task.ToolRuns)
	report.ToolRunCount = len(task.ToolRuns)

	// 3. 置信度修正
	if report.Confidence == "low" && len(report.Evidence) >= 2 {
		report.Confidence = "medium"
	}

	return report
}

// extractReportJSON 从自由文本中提取第一个 JSON 对象
func extractReportJSON(answer string) json.RawMessage {
	for i := 0; i < len(answer); i++ {
		if answer[i] == '{' {
			depth := 0
			for j := i; j < len(answer); j++ {
				switch answer[j] {
				case '{':
					depth++
				case '}':
					depth--
					if depth == 0 {
						return json.RawMessage(answer[i : j+1])
					}
				}
			}
		}
	}
	return nil
}

// parseReportJSON 解析 JSON 字段到 Report 结构体
func parseReportJSON(raw json.RawMessage, r *models.Report) {
	var parsed struct {
		Summary     string          `json:"summary"`
		RootCause   string          `json:"root_cause"`
		Confidence  string          `json:"confidence"`
		Evidence    []models.Evidence `json:"evidence"`
		Suggestions []string        `json:"suggestions"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		slog.Warn("failed to parse report JSON, using raw answer as summary", "error", err)
		r.Summary = string(raw)
		return
	}

	r.Summary = parsed.Summary
	r.RootCause = parsed.RootCause
	r.Suggestions = parsed.Suggestions

	if parsed.Confidence != "" {
		r.Confidence = parsed.Confidence
	}

	for _, e := range parsed.Evidence {
		r.Evidence = append(r.Evidence, e)
	}
}

// extractEvidence 从工具调用记录中提取证据
func extractEvidence(toolRuns []models.ToolRun) []models.Evidence {
	var evidence []models.Evidence

	for _, tr := range toolRuns {
		// 获取工具输出的文本内容
		var output map[string]any
		if err := json.Unmarshal(tr.Output, &output); err != nil {
			continue
		}

		// 收集 data 字段作为证据内容
		if data, ok := output["data"]; ok {
			content := formatToolOutput(data)
			if content != "" {
				evidence = append(evidence, models.Evidence{
					Source:   tr.ToolName,
					Content:  content,
					Relevance: "工具返回的原始数据",
				})
			}
		}

		// 如果有 message 字段也收集
		if msg, ok := output["message"].(string); ok && msg != "" {
			evidence = append(evidence, models.Evidence{
				Source:   tr.ToolName,
				Content:  msg,
				Relevance: "工具执行信息",
			})
		}
	}

	return evidence
}

// formatToolOutput 将工具输出格式化为字符串
func formatToolOutput(data any) string {
	// 如果是字符串直接返回
	if s, ok := data.(string); ok {
		return s
	}
	// 否则转为 JSON
	b, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return string(b)
}
