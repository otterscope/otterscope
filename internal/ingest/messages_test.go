package ingest

import (
	"strings"
	"testing"
)

func TestExperimentalDialectMessages(t *testing.T) {
	steps := normalizeNamed(t, "genai_experimental_chat.json")
	llm := steps["chat claude-fable-5"]
	if llm.LLM == nil {
		t.Fatal("no LLM detail")
	}

	in := llm.LLM.InputMessages
	if len(in) != 2 {
		t.Fatalf("input messages = %d, want 2 (system + user): %+v", len(in), in)
	}
	if in[0].Role != "system" || in[0].Content != "You are a careful research assistant." {
		t.Errorf("system instructions: %+v", in[0])
	}
	if in[1].Role != "user" || in[1].Content != "Summarize the latest findings." {
		t.Errorf("user message: %+v", in[1])
	}

	out := llm.LLM.OutputMessages
	if len(out) != 1 || out[0].Role != "assistant" {
		t.Fatalf("output messages: %+v", out)
	}
	if !strings.Contains(out[0].Content, "Here are the findings...") ||
		!strings.Contains(out[0].Content, `[tool_call fetch_paper({"id":42})]`) {
		t.Errorf("flattened output content: %q", out[0].Content)
	}
}

func TestOpenInferenceMessages(t *testing.T) {
	steps := normalizeNamed(t, "openai_agents_openinference.json")
	llm := steps["Response"]
	if llm.LLM == nil {
		t.Fatal("no LLM detail")
	}
	in := llm.LLM.InputMessages
	if len(in) != 1 || in[0].Role != "user" || in[0].Content != "Where is my order?" {
		t.Errorf("input messages: %+v", in)
	}
	out := llm.LLM.OutputMessages
	if len(out) != 1 || out[0].Role != "assistant" {
		t.Fatalf("output messages: %+v", out)
	}
	if !strings.Contains(out[0].Content, "[tool_call lookup_order]") {
		t.Errorf("tool call not rendered: %q", out[0].Content)
	}

	tool := steps["lookup_order"]
	if tool.Tool == nil || tool.Tool.Arguments != `{"order_id": "A-1042"}` {
		t.Errorf("tool arguments from input.value: %+v", tool.Tool)
	}
}

func TestVercelMessages(t *testing.T) {
	steps := normalizeNamed(t, "vercel_ai_sdk.json")
	llm := steps["ai.generateText.doGenerate"]
	if llm.LLM == nil {
		t.Fatal("no LLM detail")
	}
	in := llm.LLM.InputMessages
	if len(in) != 1 || in[0].Role != "user" || in[0].Content != "Where is order A-1042?" {
		t.Errorf("prompt messages: %+v", in)
	}
	out := llm.LLM.OutputMessages
	if len(out) != 1 || !strings.Contains(out[0].Content, "searchOrders") {
		t.Errorf("toolCalls output: %+v", out)
	}

	tool := steps["ai.toolCall"]
	if tool.Tool == nil || tool.Tool.Arguments != `{"query": "A-1042"}` || tool.Tool.Result != `{"status": "shipped"}` {
		t.Errorf("tool args/result: %+v", tool.Tool)
	}
}
