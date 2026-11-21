package footprint

import (
	"time"
)

const (
	MemoryLinesLimit = 200
	MemoryBytesLimit = 25600
	WarnPct          = 80
)

type Limits struct {
	MemoryLines int `json:"memory_lines"`
	MemoryBytes int `json:"memory_bytes"`
	WarnPct     int `json:"warn_pct"`
}

type Project struct {
	Key         string `json:"key"`
	MemoryLines int    `json:"memory_lines"`
	MemoryBytes int    `json:"memory_bytes"`
	Entries     int    `json:"entries"`
	PctOfLimit  int    `json:"pct_of_limit"`
}

type Global struct {
	ClaudeMDBytes           int      `json:"claude_md_bytes"`
	RulesUnconditionalBytes int      `json:"rules_unconditional_bytes"`
	RulesScopedBytes        int      `json:"rules_scoped_bytes"`
	RulesUnconditional      []string `json:"rules_unconditional"`
	RulesScoped             []string `json:"rules_scoped"`
	Skills                  int      `json:"skills"`
}

type Report struct {
	GeneratedAt time.Time `json:"generated_at"`
	Limits      Limits    `json:"limits"`
	Projects    []Project `json:"projects"`
	Global      Global    `json:"global"`
}
