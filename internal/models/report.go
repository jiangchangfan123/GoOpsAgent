package models

type Report struct {
	Summary      string     `json:"summary"`
	RootCause    string     `json:"root_cause"`
	Confidence   string     `json:"confidence"` // high / medium / low
	Evidence     []Evidence `json:"evidence"`
	Suggestions  []string   `json:"suggestions"`
	ToolRunCount int        `json:"tool_run_count"`
	GeneratedAt  string     `json:"generated_at"`
}

type Evidence struct {
	Source    string `json:"source"`    // 如 "Pod events", "container logs"
	Content   string `json:"content"`   // 证据内容
	Relevance string `json:"relevance"` // 与问题的关联
}
