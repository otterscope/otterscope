// Package model is Otterscope's domain: agent runs and their steps. All
// packages downstream of ingest depend on these types, never on OTel types.
package model

import "time"

// Status of a run or step.
type Status string

const (
	// StatusRunning means the run's root step hasn't been delivered yet.
	StatusRunning Status = "running"
	StatusOK      Status = "ok"
	StatusError   Status = "error"
)

// StepKind classifies what a step did.
type StepKind string

const (
	StepAgent   StepKind = "agent"   // agent invocation/creation
	StepLLM     StepKind = "llm"     // a model call
	StepTool    StepKind = "tool"    // a tool execution
	StepGeneric StepKind = "generic" // any other span; never dropped
)

// Run is one end-to-end agent execution — a trace, in OTel terms. Aggregate
// fields are derived from the run's steps at write time.
type Run struct {
	ID           string // trace ID, hex
	Project      string
	Service      string // resource service.name
	AgentName    string
	Status       Status
	Start        time.Time
	End          time.Time
	InputTokens  int64
	OutputTokens int64
	LLMCalls     int64
	ToolCalls    int64
	Models       string   // distinct request models, comma-joined
	CostUSD      *float64 // sum of known step costs; nil when none known
	CostPartial  bool     // true when some llm steps had no known price
	Error        string   // first step error encountered
}

// Step is one operation within a run — a span, in OTel terms.
type Step struct {
	ID        string // span ID, hex
	Project   string
	RunID     string // trace ID, hex
	ParentID  string // parent span ID, hex; "" for the root step
	Kind      StepKind
	Name      string
	Service   string
	AgentName string
	Status    Status
	Start     time.Time
	End       time.Time
	Error     string

	LLM  *LLMCall  // set when Kind == StepLLM
	Tool *ToolCall // set when Kind == StepTool
}

// Message is one conversation message on an LLM call, with multimodal parts
// flattened to text best-effort at normalization time.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// LLMCall holds the model-call details of an llm step.
type LLMCall struct {
	Provider      string // gen_ai.provider.name (new) / gen_ai.system (old)
	RequestModel  string
	ResponseModel string
	InputTokens   int64
	OutputTokens  int64
	// Subsets of the totals above; needed for accurate cost math (M3).
	CacheReadTokens     int64 // subset of InputTokens served from provider cache
	CacheCreationTokens int64 // subset of InputTokens written to provider cache
	ReasoningTokens     int64 // subset of OutputTokens spent on reasoning
	// Conversation content when the emitter opted into recording it.
	InputMessages  []Message
	OutputMessages []Message
	// CostUSD is set at ingest from the pricing table; nil = unknown model.
	CostUSD *float64
}

// ToolCall holds the tool-execution details of a tool step.
type ToolCall struct {
	Name      string
	CallID    string
	Arguments string // serialized input, when recorded
	Result    string // serialized output, when recorded
}
